'use strict';
const { readErrorText } = require('./response_read');
const { resolveInitialResponse } = require('./initial_response');
const { createLeadingText } = require('./leading_text');
const { createFinalValidation } = require('./final_validation');
const { createDiagnosticProgress } = require('./diagnostic_progress');
const { createStreamWatch } = require('./stream_limits');
const { startHeartbeat, waitForDrain } = require('./response_flow');
const { previewUsage, writeSuccessfulTerminal } = require('./stream_completion');
const { explicitSnapshotDelta } = require('./dedupe');
const { flushThinkingState } = require('./sse_thinking');
const { createVisibleGuard, filterVisible } = require('../helpers/stream-tool-sieve/visible_guard');
const { needsToolFinalization, validatedToolText } = require('./tool_output');

const {
  createToolSieveState,
  processToolSieveChunk,
  flushToolSieve,
  parseStandaloneToolCalls,
} = require('../helpers/stream-tool-sieve');
const { fetchAppStream } = require('../shared/app-transport');
const { createStreamLifecycle } = require('./stream_lifecycle');
const { writeOpenAIError, writeOpenAIErrorWithCode } = require('./error_shape');
const { parseChunkForContent } = require('./sse_parse');
const { mergeReasoningForUsage } = require('./token_usage');
const { createUsageAccounting } = require('./usage_accounting');
const {
  resolveToolcallPolicy,
  toolChoiceViolation,
} = require('./toolcall_policy');
const { createChatCompletionEmitter, createDeltaCoalescer } = require('./stream_emitter');
const {
  asString,
  fetchStreamPrepare,
  fetchStreamSwitch,
  fetchStreamValidation,
  relayPreparedFailure,
} = require('./http_internal');
const {
  CUT_NONE,
  CUT_PROMPT_ECHO,
  flushContentPolicy,
  createContentPolicy,
  feedContentPolicy,
  feedContentPolicyTail,
  contentPolicyCut,
} = require('./content_policy');
const {
  createMarkerNormalizer,
} = require('./marker');

const {
  isTransientUpstreamStatus,
  isAccountAuthFailureStatus,
  TRANSIENT_SWITCH_MAX_ATTEMPTS,
  switchOptionsForStatus, isUnavailableEmptyOutput, switchOptionsForFailure,
  isTransientUpstreamMessage,
  EMPTY_OUTPUT_RETRY_MAX_ATTEMPTS,
  clonePayloadForOutputRetry,
} = require('./stream_retry_policy');
const {
  createContinueState,
  observeContinueState,
  shouldAutoContinue,
  AUTO_CONTINUE_MAX_ROUNDS,
  prepareContinueStateForNextRound,
} = require('./stream_continue');
const { upstreamEmptyOutputDetail } = require('./stream_failure');
const { createStreamReport } = require('./stream_report');
const { fetchAccountCompletion } = require('./completion_account_retry');
const { createUpstreamConnection } = require('./upstream_connection');

const DEEPSEEK_COMPLETION_URL = 'https://chat.deepseek.com/api/v0/chat/completion';
const DEEPSEEK_CONTINUE_URL = 'https://chat.deepseek.com/api/v0/chat/continue';
async function handleVercelStream(req, res, rawBody, payload, appFetch = fetchAppStream) {
  let leaseCompletion = {};
  const delivery = { delivery_version: 1, tools_written: false, response_written: false };
  let partialCompletion = () => ({});
  let usageAccounting;
  let streamStarted = false;
  let outputWriteError = '';
  const report = createStreamReport();
  const { fail: sendFailedChunk, finishFailure: finishStreamFailure } = report;
  const lifecycle = createStreamLifecycle(req, res, () => report.snapshot({
    ...(leaseCompletion.finish_reason ? leaseCompletion : partialCompletion()), ...usageAccounting?.metadata(), ...delivery,
    ...(outputWriteError && !delivery.response_written ? { finish_reason: 'error', error: outputWriteError, error_code: 'client_write_error', error_status: 499 } : {}),
  }));
  const releaseLease = lifecycle.release;
  const watch = createStreamWatch();
  let stopHeartbeat = () => {};
  try {
    if (lifecycle.closed) return;
    const prep = await fetchStreamPrepare(req, rawBody, lifecycle.signal, watch.deadline);
    prep.body = prep.body && typeof prep.body === 'object' ? prep.body : {};
    lifecycle.trackLease(asString(prep.body.lease_id));
    if (lifecycle.closed) return;
    if (!prep.ok) {
      relayPreparedFailure(res, prep);
      return;
    }

    const model = asString(prep.body.model) || asString(payload.model);
    const responseID = asString(prep.body.session_id) || `chatcmpl-${Date.now()}`;
    const leaseID = asString(prep.body.lease_id);
    let deepseekToken = asString(prep.body.deepseek_token);
    const initialPowHeader = asString(prep.body.pow_header);
    let completionPayload = prep.body.payload && typeof prep.body.payload === 'object' ? prep.body.payload : null;
    let finalPrompt = asString(prep.body.final_prompt);
    usageAccounting = createUsageAccounting(prep.body);

    const thinkingEnabled = toBool(prep.body.thinking_enabled);
    const toolPolicy = resolveToolcallPolicy(prep.body, payload.tools);
    const toolNames = toolPolicy.toolNames;
    const stripReferenceMarkers = true;

    if (!model || !leaseID || !deepseekToken || !initialPowHeader || !completionPayload) {
      writeOpenAIError(res, 500, 'invalid vercel prepare response');
      return;
    }

    const connection = createUpstreamConnection({ prepared: prep.body, req, leaseID, lifecycle, deadline: watch.deadline, appFetch });
    const fetchCompletion = (bodyPayload) => fetchAccountCompletion(bodyPayload, {
      readOptions: { deadline: watch.deadline, signal: lifecycle.signal },
      fetch: (body) => connection.fetch(DEEPSEEK_COMPLETION_URL, body, connection.powHeader),
      closed: () => lifecycle.closed,
      canSwitch: () => !streamStarted || !answerDelivered,
      switch: (options) => switchAccount(options),
      adopt: (switched) => {
        applySwitchedPayload(switched);
        if (streamStarted) resetForRegeneration();
      },
      fail: async (failure) => {
        leaseCompletion = { finish_reason: 'error', error: failure.message, error_code: failure.code };
        if (streamStarted) {
          sendFailedChunk(res, failure.status, failure.message, failure.code);
          if (!res.writableEnded && !res.destroyed) res.end();
        } else writeOpenAIErrorWithCode(res, failure.status, failure.message, failure.code);
        await releaseLease();
      },
    });
    let activeDeepSeekSessionID = responseID;
    const fetchContinue = async (messageID) => {
      try {
        return await connection.fetch(DEEPSEEK_CONTINUE_URL, {
          chat_session_id: activeDeepSeekSessionID,
          message_id: messageID,
          fallback_to_resume: true,
        }, '');
      } catch (err) {
        console.warn('[vercel_stream] continuation failed', err.message);
        return null;
      }
    };
    // applySwitchedPayload 把 __stream_switch 返回的新账号 payload / token / PoW
    // / session 应用到当前流式请求，供首包与重试阶段的临时上游故障切号复用。
    const applySwitchedPayload = (switched) => {
      completionPayload = switched.body.payload;
      finalPrompt = asString(switched.body.final_prompt) || finalPrompt;
      activeDeepSeekSessionID = asString(switched.body.session_id) || activeDeepSeekSessionID;
      connection.adopt(switched.body);
      usageAccounting.reset({ ...switched.body, final_prompt: finalPrompt });
    };

    // Let an in-flight Go control operation settle before releasing its lease,
    // but do not start a new one or replay completion after disconnect.
    const switchAccount = async (options) => {
      if (lifecycle.closed) return null;
      const switched = await fetchStreamSwitch(req, leaseID, options, lifecycle.signal, watch.deadline);
      return lifecycle.closed ? null : switched;
    };

    let completionRes = await fetchCompletion(completionPayload);
    if (completionRes === null) {
      return;
    }
    if (lifecycle.closed) {
      return;
    }

    completionRes = await resolveInitialResponse(completionRes, {
      res, readOptions: { deadline: watch.deadline, signal: lifecycle.signal }, prepared: prep.body,
      switchAccount, adopt: applySwitchedPayload, fetchCompletion, closed: () => lifecycle.closed,
    });
    if (!completionRes) return;

    if (lifecycle.closed) return;
    res.statusCode = 200;
    res.setHeader('Content-Type', 'text/event-stream');
    res.setHeader('Cache-Control', 'no-cache, no-transform');
    res.setHeader('Connection', 'keep-alive');
    res.setHeader('X-Accel-Buffering', 'no');
    if (typeof res.flushHeaders === 'function') {
      res.flushHeaders();
    }

    streamStarted = true;
    stopHeartbeat = startHeartbeat(res);
    const created = Math.floor(Date.now() / 1000);
    let currentType = thinkingEnabled ? 'thinking' : 'text';
    let thinkingText = '';
    let rawThinkingText = '';
    let thinkingGuard = createVisibleGuard(prep.body.tool_marker, true, finalPrompt);
    let detectionThinkingText = '';
    const rawReasoning = () => mergeReasoningForUsage(rawThinkingText, detectionThinkingText, thinkingText);
    let outputText = '';
    partialCompletion = () => ({ finish_reason: 'stopped', usage: usageAccounting.build(rawReasoning(), outputText), thinking: rawReasoning(), content: outputText, error: '' });
    let hasVisibleOutput = false;
    let deferredToolText = '';
    // Content policy is shared with Go; regeneration resets attempt-local state.
    let contentPolicy = createContentPolicy(finalPrompt);
    const toolSieveEnabled = toolPolicy.toolSieveEnabled;
    let toolSieveState = createToolSieveState();
    toolSieveState.deferMalformed = toolSieveState.cleanVisible = toolSieveState.stripReferences = true;
    // 调用者专属工具调用标识（由 Go 侧按 API Key 派生并随 prepare 下发）。
    // 上游只会输出该标识，而 sieve / 独立解析 / 复读守卫都只认 EPSE，
    // 因此所有流入文本都要先经它有状态地归一化回 EPSE（跨分片标识会被
    // 暂存到下一片，避免半截标识泄漏成正文）。
    let markerNormalizer = createMarkerNormalizer(prep.body.tool_marker);
    let toolCallsEmitted = false;
    let toolCallsDetected = false;
    let toolCallsDoneEmitted = false;
    const decoder = new TextDecoder();
    let buffered = '';
    let ended = false;
    let lastEmptyDetail = null;
    let answerDelivered = false;
    const leadingText = createLeadingText();
    const finalValidation = createFinalValidation();
    const diagnosticProgress = createDiagnosticProgress({ enabled: prep.body.diagnostic_progress === true, req, leaseID, deadline: watch.finalDeadline, signal: lifecycle.signal, diagnostic: report.diagnostic, read: () => ({ content: outputText, thinking: rawReasoning() }) });
    const holdToolText = () => toolPolicy.holdText;
    const { sendFrame, sendDeltaFrame: emitDeltaFrame } = createChatCompletionEmitter({
      res,
      sessionID: responseID,
      created,
      model,
      isClosed: () => lifecycle.closed,
      onWriteError: error => { outputWriteError = error.message; },
    });
    const sendDeltaFrame = (delta) => {
      if (typeof delta.content === 'string') {
        delta.content = leadingText.write(delta.content);
        if (!delta.content) delete delta.content;
      }
      if (!Object.keys(delta).length) return;
      const written = emitDeltaFrame(delta);
      if (written && (asString(delta.content).trim() || delta.tool_calls?.length)) answerDelivered = true;
      if (written && asString(delta.content).trim()) report.mark('first_visible_ms');
      if (written && delta.tool_calls?.length) delivery.tools_written = true;
      return written;
    };
    const deltaCoalescer = createDeltaCoalescer({ sendDeltaFrame });
    const resetForRegeneration = () => {
      report.regenerate(lastEmptyDetail);
      finalValidation.reset();
      diagnosticProgress.reset();
      leadingText.reset();
      currentType = thinkingEnabled ? 'thinking' : 'text';
      contentPolicy = createContentPolicy(finalPrompt);
      outputText = '';
      hasVisibleOutput = false;
      deferredToolText = '';
      toolCallsDetected = false;
      detectionThinkingText = '';
      rawThinkingText = '';
      thinkingGuard = createVisibleGuard(prep.body.tool_marker, true, finalPrompt);
      lastEmptyDetail = null;
      toolSieveState = createToolSieveState();
      toolSieveState.deferMalformed = toolSieveState.cleanVisible = toolSieveState.stripReferences = true;
      markerNormalizer = createMarkerNormalizer(prep.body.tool_marker);
    };

    const finish = async (reason, options = {}) => {
      report.mark('upstream_done_ms', true);
      if (ended) {
        return true;
      }
      if (lifecycle.closed || res.writableEnded || res.destroyed) {
        ended = true;
        leaseCompletion = {
          finish_reason: 'stopped',
          usage: usageAccounting.build(rawReasoning(), outputText),
          thinking: rawReasoning(),
          content: outputText,
          error: '',
        };
        await releaseLease();
        return true;
      }
      if (reason === 'content_filter') {
        deltaCoalescer.flush(); ended = true;
        const detail = upstreamEmptyOutputDetail(true, '', '');
        leaseCompletion = { finish_reason: 'error', usage: null, thinking: rawReasoning(), content: outputText, error: detail.message, error_code: detail.code };
        await finishStreamFailure(res, releaseLease, detail); return true;
      }
      if (contentPolicyCut(contentPolicy) === CUT_PROMPT_ECHO || thinkingGuard.echoed) {
        lastEmptyDetail = { status: 502, code: 'upstream_prompt_echo', message: 'Upstream repeated generated instructions instead of producing an answer.' };
        if (options.deferEmpty && !answerDelivered) return false;
        ended = true;
        leaseCompletion = { finish_reason: 'error', usage: null, thinking: rawReasoning(), content: '', error: lastEmptyDetail.message, error_code: lastEmptyDetail.code };
        sendFailedChunk(res, 502, lastEmptyDetail.message, lastEmptyDetail.code);
        await releaseLease();
        if (!res.writableEnded && !res.destroyed) res.end();
        return true;
      }
      const boundaryTail = flushThinkingState(toolSieveState);
      if (boundaryTail) {
        detectionThinkingText += boundaryTail;
        if (thinkingEnabled) { rawThinkingText += boundaryTail; const visible = thinkingGuard.write(boundaryTail); thinkingText += visible; deltaCoalescer.append('reasoning_content', visible); }
      }
      const thinkingTail = thinkingGuard.flush();
      if (thinkingTail) { thinkingText += thinkingTail; deltaCoalescer.append('reasoning_content', thinkingTail); }
      deltaCoalescer.flush();
      // Unconfirmed prompt prefixes are literal EOF text and already passed
      // the role guard; marker-normalizer tails cannot form a role delimiter.
      const markerTail = flushContentPolicy(contentPolicy) + markerNormalizer.flush();
      if (markerTail) {
        outputText += markerTail;
        if (markerTail.trim() !== '') {
          hasVisibleOutput = true;
        }
        feedContentPolicyTail(contentPolicy, markerTail);
      }
      let detected = parseStandaloneToolCalls(outputText, toolNames);
      // Match the Go fallback: only use reasoning-channel calls when no body
      // answer exists. Hidden reasoning remains internal and is never emitted.
      if (detected.length === 0 && !hasVisibleOutput && detectionThinkingText) {
        const normalizer = createMarkerNormalizer(prep.body.tool_marker);
        detected = parseStandaloneToolCalls(normalizer.write(detectionThinkingText) + normalizer.flush(), toolNames);
      }
      let validatedCalls = null, validatedUsage = null, validatedText = null;
      if ((detected.length || needsToolFinalization(outputText, detectionThinkingText, prep.body, toolSieveState)) && (hasVisibleOutput || detected.length || detectionThinkingText)) {
        try {
          const validation = await finalValidation.run(() => report.measure('validation_ms', () => fetchStreamValidation(req, leaseID, outputText, rawReasoning(), detectionThinkingText, lifecycle.signal, watch.finalDeadline, { ...report.diagnostic(), ...usageAccounting.validation() }, prep.body.tool_repair_timeout_ms)));
          if (!validation.ok) {
            const error = validation.body.error || {};
            if (options.deferEmpty && !answerDelivered && ['invalid_tool_arguments', 'tool_choice_violation', 'upstream_empty_output', 'upstream_unavailable'].includes(error.code)) {
              lastEmptyDetail = { status: validation.status, message: error.message || 'Tool validation failed', code: error.code };
              return false;
            }
            ended = true;
            leaseCompletion = { finish_reason: 'error', usage: null, thinking: rawReasoning(), content: outputText, error: error.message || 'Tool validation failed' };
            sendFailedChunk(res, validation.status, leaseCompletion.error, error.code || 'invalid_tool_arguments');
            await releaseLease(); if (!res.writableEnded && !res.destroyed) res.end(); return true;
          }
          validatedCalls = validation.body.tool_calls || [];
          validatedUsage = validation.body.usage;
          validatedText = typeof validation.body.content === 'string' ? validation.body.content : null;
          detected = validatedCalls.map(call => ({ name: call.function.name }));
        } catch (error) {
          if (lifecycle.closed) {
            ended = true; leaseCompletion = { finish_reason: 'stopped', usage: usageAccounting.build(rawReasoning(), outputText), thinking: rawReasoning(), content: outputText, error: '' };
            await releaseLease(); return true;
          }
          ended = true;
          leaseCompletion = { finish_reason: 'error', usage: null, thinking: rawReasoning(), content: outputText, error: 'Tool validation unavailable: ' + error.message };
          sendFailedChunk(res, 502, leaseCompletion.error, 'tool_validation_unavailable');
          await releaseLease(); if (!res.writableEnded && !res.destroyed) res.end(); return true;
        }
      }
      const violation = (detected.length > 0 || toolCallsEmitted || hasVisibleOutput)
        ? toolChoiceViolation(detected, toolPolicy.choice) : '';
      if (violation) {
        if (options.deferEmpty && !answerDelivered) {
          lastEmptyDetail = { status: 422, message: violation, code: 'tool_choice_violation' };
          return false;
        }
        ended = true;
        leaseCompletion = { finish_reason: 'error', usage: null, thinking: rawReasoning(), content: '', error: violation };
        sendFailedChunk(res, 422, violation, 'tool_choice_violation');
        await releaseLease();
        if (!res.writableEnded && !res.destroyed) res.end();
        return true;
      }
      if (holdToolText()) { deltaCoalescer.append('content', validatedText ?? validatedToolText(outputText, toolNames, detected.length > 0)); deltaCoalescer.flush(); }
      if (!holdToolText() && !detected.length && !toolCallsEmitted && deferredToolText) { deltaCoalescer.append('content', filterVisible(deferredToolText)); deferredToolText = ''; }
      if (detected.length > 0 && !toolCallsDoneEmitted) {
        if (sendDeltaFrame({ tool_calls: validatedCalls })) {
          toolCallsEmitted = true;
          toolCallsDoneEmitted = true;
        }
      } else if (toolSieveEnabled && !holdToolText()) {
        const tailEvents = markerTail
          ? processToolSieveChunk(toolSieveState, markerTail, toolNames).concat(flushToolSieve(toolSieveState, toolNames))
          : flushToolSieve(toolSieveState, toolNames);
        for (const evt of tailEvents) {
          if (evt.type === 'tool_calls') continue;
          if (evt.text) {
            // 已发出 tool_calls 后，收尾阶段剩余的工具块后正文同样丢弃，与流式处理一致。
            if (toolCallsEmitted) {
              continue;
            }
            deltaCoalescer.append('content', evt.deferred ? filterVisible(evt.text) : evt.text);
          }
        }
        deltaCoalescer.flush();
      }
      if (detected.length > 0 || toolCallsEmitted) {
        reason = 'tool_calls';
      }
      if (detected.length === 0 && !toolCallsEmitted && !hasVisibleOutput) {
        if (options.deferEmpty && reason !== 'content_filter') {
          lastEmptyDetail = upstreamEmptyOutputDetail(reason === 'content_filter', outputText, thinkingText);
          return false;
        }
        ended = true;
        const detail = lastEmptyDetail || upstreamEmptyOutputDetail(reason === 'content_filter', outputText, thinkingText);
        leaseCompletion = { finish_reason: reason, usage: null, thinking: rawReasoning(), content: outputText, error: detail.message };
        sendFailedChunk(res, detail.status, detail.message, detail.code);
        await releaseLease();
        if (!res.writableEnded && !res.destroyed) {
          res.end();
        }
        return true;
      }
      ended = true;
      let finalUsage = usageAccounting.build(rawReasoning(), outputText, validatedUsage);
      finalUsage = await previewUsage(req, leaseID, report.snapshot({ finish_reason: reason, usage: finalUsage, thinking: rawReasoning(), content: outputText, error: '', ...usageAccounting.metadata() }), finalUsage, usageAccounting);
      await writeSuccessfulTerminal(res, sendFrame, {
        id: responseID,
        object: 'chat.completion.chunk',
        created,
        model,
        choices: [{ delta: {}, index: 0, finish_reason: reason }],
        usage: finalUsage,
      });
      delivery.response_written = true;
      leaseCompletion = {
        finish_reason: reason,
        usage: finalUsage,
        thinking: rawReasoning(),
        content: outputText,
        error: '',
      };
      await releaseLease();
      if (!res.writableEnded && !res.destroyed) {
        res.end();
      }
      return true;
    };

    const processStream = async (initialResponse, allowDeferEmpty) => {
      let currentResponse = initialResponse;
      let continueState = createContinueState(activeDeepSeekSessionID);
      let continueRounds = 0;
      let contentCut = CUT_NONE;
      while (true) {
        // Continuations retain delimiter/CDATA state; regeneration resets it.
        lifecycle.reader = currentResponse.body.getReader();
        buffered = '';
        let streamEnded = false;
        try {
          // eslint-disable-next-line no-constant-condition
          while (true) {
            if (lifecycle.closed) {
              await finish('stop');
              return { terminal: true, retryable: false };
            }
            await waitForDrain(res);
            const { value, done } = await watch.read(lifecycle.reader);
            if (done) {
              break;
            }
            buffered += decoder.decode(value, { stream: true });
            const lines = buffered.split('\n');
            buffered = lines.pop() || '';

            for (const rawLine of lines) {
              const line = rawLine.trim();
              if (!line.startsWith('data:')) {
                continue;
              }
              const dataStr = line.slice(5).trim();
              if (!dataStr) {
                continue;
              }
              if (dataStr === '[DONE]') {
                continueState.sawDone = true;
                streamEnded = true;
                break;
              }
              let chunk;
              try {
                chunk = JSON.parse(dataStr);
              } catch (_err) {
                continue;
              }
              observeContinueState(continueState, chunk);
              const parsed = parseChunkForContent(chunk, thinkingEnabled, currentType, stripReferenceMarkers, toolSieveState);
              if (!parsed.parsed) {
                continue;
              }
              currentType = parsed.newType;
              if (parsed.detectionThinking || parsed.parts.some(part => part.text)) continueState.sawContent = true;
              if (parsed.errorMessage) {
                // 临时上游错误（如“服务器暂时不可用”）可重试：交给外层重试/切号
                // 循环处理，而不是伪装成 content_filter 直接终止。
                deltaCoalescer.flush();
                if (isTransientUpstreamMessage(parsed.errorMessage) && !answerDelivered && allowDeferEmpty) {
                  lastEmptyDetail = { status: 503, message: parsed.errorMessage, code: 'upstream_unavailable' };
                  return { terminal: false, retryable: true, responseMessageID: continueState.responseMessageID, interrupted: false };
                }
                deltaCoalescer.flush();
                ended = true;
                leaseCompletion = { finish_reason: 'error', usage: null, thinking: rawReasoning(), content: outputText, error: parsed.errorMessage };
                sendFailedChunk(res, 502, parsed.errorMessage, 'upstream_error');
                await releaseLease();
                if (!res.writableEnded && !res.destroyed) res.end();
                return { terminal: true, retryable: false };
              }
              if (parsed.contentFilter) {
                return { terminal: await finish('content_filter'), retryable: false };
              }

              if (parsed.detectionThinking) {
                detectionThinkingText += parsed.snapshot ? explicitSnapshotDelta(detectionThinkingText, parsed.detectionThinking) : parsed.detectionThinking;
              }
              for (const p of parsed.parts) {
                if (!p.text) {
                  continue;
                }
                if (p.type === 'thinking') {
                  if (thinkingEnabled) {
                    const rawTrimmed = parsed.snapshot ? explicitSnapshotDelta(rawThinkingText, p.text) : p.text;
                    rawThinkingText += rawTrimmed;
                    const trimmed = thinkingGuard.write(rawTrimmed);
                    if (!trimmed) {
                      continue;
                    }
                    thinkingText += trimmed;
                    deltaCoalescer.append('reasoning_content', trimmed);
                  }
                } else {
                  // 先归一化再进入复读守卫 / 累计文本 / sieve：三者都只认 EPSE。
                  // 思考通道由独立的可见性 guard 归一化并拦截工具格式。
                  const normalizedText = markerNormalizer.write(p.text);
                  if (!normalizedText) {
                    continue;
                  }
                  const trimmed = parsed.snapshot ? explicitSnapshotDelta(outputText, normalizedText) : normalizedText;
                  if (!trimmed) {
                    continue;
                  }
                  // 内容策略可能在此截断正文通道：编造的 `[Tool]:` 角色标记及其后
                  // 全是虚构对话，复读死循环之后的输出也已退化。两种情况都只保留
                  // 截断点之前的正文。
                  const safeText = feedContentPolicy(contentPolicy, trimmed);
                  if (!safeText) {
                    continue;
                  }
                  outputText += safeText;
                  if (!hasVisibleOutput) {
                    hasVisibleOutput = safeText.trim() !== '';
                  }
                  if (holdToolText()) continue;
                  if (!toolSieveEnabled) {
                    deltaCoalescer.append('content', safeText);
                    continue;
                  }
                  const events = processToolSieveChunk(toolSieveState, safeText, toolNames);
                  for (const evt of events) {
                    if (evt.deferred) { deferredToolText += evt.text || ''; continue; }
                    if (evt.type === 'tool_call_deltas') continue;
                    // Deliver only Go-finalized calls. This also covers
                    // continuation requests without a formal tools catalog.
                    if (evt.type === 'tool_calls') { report.mark('tool_detected_ms'); toolCallsDetected = true; continue; }
                    if (evt.text) {
                      // 已发出 tool_calls 后，工具调用块之后追加的尾巴正文不再透传
                      // （OpenAI 规范中 tool_calls 回合的 content 应为空）。
                      if (toolCallsEmitted || toolCallsDetected) {
                        continue;
                      }
                      deltaCoalescer.append('content', evt.text);
                    }
                  }
                }
              }
              diagnosticProgress.observe(outputText, detectionThinkingText, toolSieveState);
              if (parsed.finished) {
                streamEnded = true;
                break;
              }
              if (contentPolicyCut(contentPolicy) !== CUT_NONE) {
                // 上游内容规则（复读死循环 / 编造的 `[Tool]:` 角色标记）触发：
                // 停止读取上游，保留已生成内容交给正常收尾（工具筛分 /
                // finish 帧 / [DONE]），仍是 200。
                contentCut = contentPolicyCut(contentPolicy);
                streamEnded = true;
                break;
              }
              if (streamEnded) {
                break;
              }
            }
            if (streamEnded) {
              break;
            }
          }
        } catch (err) {
          if (err.code === 'output_limit_exceeded') {
            ended = true; leaseCompletion = { finish_reason: 'error', error: err.message, error_code: err.code };
            await finishStreamFailure(res, releaseLease, { status: 502, message: err.message, code: err.code });
            return { terminal: true, retryable: false };
          }
          if (lifecycle.closed || lifecycle.signal.aborted) {
            await finish('stop');
            return { terminal: true, retryable: false, interrupted: false };
          }
          // Hidden reasoning and held protocol prefixes are partial output too.
          // Never finalize executable calls from a stream without a terminal.
          continueState.sawContent ||= outputText.length > 0 || rawThinkingText.length > 0 || detectionThinkingText.length > 0;
          if (!continueState.sawContent) break;
        } finally {
          // Release the old upstream stream before retrying or waiting for a
          // different account. Otherwise an error event can leave its HTTP
          // stream alive and keep consuming upstream concurrency.
          const finishedReader = lifecycle.reader;
          lifecycle.reader = null;
          if (finishedReader) {
            try { await finishedReader.cancel(); }
            catch (err) { console.warn('[vercel_stream] reader cancel failed', err.message); }
            finishedReader.releaseLock();
          }
        }

        if (contentCut !== CUT_NONE) {
          // 复读死循环 / 编造的 `[Tool]:` 角色标记已打断：主动取消上游读取，
          // 且不再走 auto-continue，否则续答只会立刻重新进入同一种退化输出。
          break;
        }

        if (shouldAutoContinue(continueState) && continueRounds < AUTO_CONTINUE_MAX_ROUNDS) {
          continueRounds += 1;
          const nextRes = await fetchContinue(continueState.responseMessageID);
          if (lifecycle.closed || lifecycle.signal.aborted) {
            return { terminal: true, retryable: false };
          }
          if (!nextRes || !nextRes.ok || !nextRes.body) {
            try { await nextRes?.body?.cancel(); }
            catch (err) { console.warn('[vercel_stream] failed continuation cancel failed', err.message); }
            return { terminal: false, retryable: false, interrupted: true, responseMessageID: continueState.responseMessageID };
          }
          continueState = prepareContinueStateForNextRound(continueState);
          currentResponse = nextRes;
          continue;
        }
        if (shouldAutoContinue(continueState) || (!continueState.finished && !continueState.sawDone && continueState.sawContent)) {
          return { terminal: false, retryable: false, interrupted: true, responseMessageID: continueState.responseMessageID };
        }
        break;
      }

      const terminal = await finish('stop', { deferEmpty: allowDeferEmpty });
      return { terminal, retryable: !terminal && allowDeferEmpty, responseMessageID: continueState.responseMessageID, interrupted: false };
    };

    let retryAttempts = 0;
    let accountSwitchAttempted = false;
    let transientSwitchAttempts = 0;
    while (true) {
      if (!completionRes.ok || !completionRes.body) {
        const status = completionRes.ok ? 502 : completionRes.status || 502;
        const message = completionRes.body && !completionRes.bodyUsed ? await readErrorText(completionRes, { deadline: watch.deadline, signal: lifecycle.signal }) : (lastEmptyDetail && lastEmptyDetail.message) || '';
        if (!isTransientUpstreamStatus(status) && !isAccountAuthFailureStatus(status) && status !== 429) {
          ended = true;
          leaseCompletion = { finish_reason: 'error', usage: null, thinking: rawReasoning(), content: outputText, error: message };
          sendFailedChunk(res, status, message, 'upstream_error');
          await releaseLease();
          if (!res.writableEnded && !res.destroyed) res.end();
          return;
        }
        lastEmptyDetail = { status, message, code: isAccountAuthFailureStatus(status) ? 'account_unauthorized' : 'upstream_unavailable' };
      }
      const pendingTransient = lastEmptyDetail && ['upstream_unavailable', 'account_unauthorized'].includes(lastEmptyDetail.code);
      const allowDeferEmpty = retryAttempts < EMPTY_OUTPUT_RETRY_MAX_ATTEMPTS || !accountSwitchAttempted || pendingTransient;
      const processed = completionRes.ok && completionRes.body
        ? await processStream(completionRes, allowDeferEmpty)
        : { terminal: false, retryable: true, interrupted: false };
      const isUpstreamUnavailable = lastEmptyDetail && lastEmptyDetail.code === 'upstream_unavailable';
      const unavailableEmpty = prep.body.managed_account === true && isUnavailableEmptyOutput(lastEmptyDetail);
      const isAccountUnauthorized = lastEmptyDetail && lastEmptyDetail.code === 'account_unauthorized';
      const isInputContentBlocked = lastEmptyDetail && lastEmptyDetail.code === 'input_content_blocked';
      if (processed.terminal) {
        return;
      }
      if (processed.interrupted) {
        // A failed continuation cannot finalize an executable partial batch.
        deltaCoalescer.flush();
        ended = true;
        leaseCompletion = { finish_reason: 'upstream_interrupted', usage: null, thinking: rawReasoning(), content: outputText, error: 'Upstream stream interrupted before completion.' };
        await releaseLease();
        sendFailedChunk(res, 502, 'Upstream stream interrupted before completion.', 'upstream_interrupted');
        if (!res.writableEnded && !res.destroyed) res.end();
        return;
      }
      if (!processed.retryable) {
        await finish('stop');
        return;
      }
      if (retryAttempts >= EMPTY_OUTPUT_RETRY_MAX_ATTEMPTS || unavailableEmpty) {
        if (lastEmptyDetail?.code === 'upstream_prompt_echo') { await finish('stop'); return; }
        if (isInputContentBlocked) {
          await finish('stop');
          return;
        }
        if (isUpstreamUnavailable || isAccountUnauthorized) {
          if (answerDelivered || prep.body.failover_budget_managed !== true && !unavailableEmpty && transientSwitchAttempts >= TRANSIENT_SWITCH_MAX_ATTEMPTS) {
            await finish('stop');
            return;
          }
          transientSwitchAttempts += 1;
          const switched = await switchAccount(switchOptionsForFailure(lastEmptyDetail));
          if (!switched) return;
          if (switched.ok && switched.body && switched.body.payload && typeof switched.body.payload === 'object') {
            applySwitchedPayload(switched);
            resetForRegeneration();
            completionRes = await fetchCompletion(completionPayload);
            if (completionRes === null) {
              return;
            }
            continue;
          }
          await finish('stop');
          return;
        }
        if (!accountSwitchAttempted) {
          accountSwitchAttempted = true;
          const switched = await switchAccount();
          if (!switched) return;
          if (switched.ok && switched.body && switched.body.payload && typeof switched.body.payload === 'object') {
            applySwitchedPayload(switched);
            resetForRegeneration();
            completionRes = await fetchCompletion(completionPayload);
            if (completionRes === null) {
              return;
            }
            continue;
          }
        }
        await finish('stop');
        return;
      }
      retryAttempts += 1;
      console.info('[openai_empty_retry] attempting synthetic retry', {
        surface: 'chat.completions',
        stream: true,
        retry_attempt: retryAttempts,
        parent_message_id: processed.responseMessageID || 0,
      });
      const retryPayload = clonePayloadForOutputRetry(completionPayload, processed.responseMessageID, lastEmptyDetail, outputText || detectionThinkingText);
      const retryPowHeader = await connection.refreshPow('retry');
      if (!retryPowHeader) {
        if (connection.powFailure && !lifecycle.closed) {
          const switched = await switchAccount(connection.powFailure.options);
          if (switched?.ok && switched.body?.payload) {
            applySwitchedPayload(switched);
            resetForRegeneration();
            completionRes = await fetchCompletion(completionPayload);
            if (completionRes) continue;
          } else {
            lastEmptyDetail = connection.powFailure;
            await finish('stop');
          }
        }
        return;
      }
      connection.powHeader = retryPowHeader;
      usageAccounting.retry(retryPayload);
      completionRes = await fetchCompletion(retryPayload);
      if (completionRes === null) {
        return;
      }
      if (!completionRes.ok || !completionRes.body) {
        const retryStatus = completionRes.ok ? 500 : completionRes.status || 500;
        if (isTransientUpstreamStatus(retryStatus) || isAccountAuthFailureStatus(retryStatus)) {
          const retryDetail = completionRes.body ? await readErrorText(completionRes, { deadline: watch.deadline, signal: lifecycle.signal }) : '';
          lastEmptyDetail = {
            status: retryStatus,
            message: retryDetail,
            code: isAccountAuthFailureStatus(retryStatus) ? 'account_unauthorized' : 'upstream_unavailable',
          };
          continue;
        }
        await finish('stop');
        return;
      }
      resetForRegeneration();
    }
  } catch (error) {
    if (error.code === 'client_write_error' && !lifecycle.closed) outputWriteError = error.message;
    if (!lifecycle.closed && !res.writableEnded && !res.destroyed) {
      const code = ['output_limit_exceeded', 'client_write_error'].includes(error.code) ? error.code : 'upstream_unavailable';
      const message = ['output_limit_exceeded', 'client_write_error'].includes(code) ? error.message : 'Upstream request failed or timed out.';
      leaseCompletion = { ...partialCompletion(), finish_reason: 'error', error: message, error_code: code };
      if (streamStarted) { sendFailedChunk(res, 502, message, code); res.end(); }
      else writeOpenAIErrorWithCode(res, 502, message, code);
    }
  } finally {
    stopHeartbeat();
    await lifecycle.dispose();
  }
}

function toBool(v) {
  return v === true;
}

module.exports = {
  handleVercelStream,
};
