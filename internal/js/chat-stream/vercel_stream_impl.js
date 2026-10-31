'use strict';

// Coordinate one prepared stream; policy, continuation and probes live in dedicated modules.

const {
  createToolSieveState,
  processToolSieveChunk,
  flushToolSieve,
  parseStandaloneToolCalls,
  formatOpenAIStreamToolCalls,
} = require('../helpers/stream-tool-sieve');
const { BASE_HEADERS } = require('../shared/deepseek-constants');
const { serializeAppPayload } = require('../shared/deepseek-wire');
const { fetchAppStream } = require('../shared/app-transport');
const { createStreamLifecycle } = require('./stream_lifecycle');
const { writeOpenAIError, writeOpenAIErrorWithCode } = require('./error_shape');
const { parseChunkForContent, isCitation } = require('./sse_parse');
const { buildUsage } = require('./token_usage');
const {
  resolveToolcallPolicy,
  toolChoiceViolation,
  formatIncrementalToolCallDeltas,
  filterIncrementalToolCallDeltasByAllowed,
  resetStreamToolCallState,
} = require('./toolcall_policy');
const { createChatCompletionEmitter, createDeltaCoalescer } = require('./stream_emitter');
const {
  asString,
  isAbortError,
  fetchStreamPrepare,
  fetchStreamPow,
  fetchStreamSwitch,
  fetchStreamValidation,
  relayPreparedFailure,
} = require('./http_internal');
const {
  trimContinuationOverlap,
} = require('./dedupe');
const {
  CUT_NONE,
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
  switchOptionsForStatus,
  isTransientUpstreamMessage,
  EMPTY_OUTPUT_RETRY_MAX_ATTEMPTS,
  TOOL_OUTPUT_RETRY_SUFFIX,
  usagePromptWithEmptyOutputRetry,
  clonePayloadForEmptyOutputRetry,
} = require('./stream_retry_policy');
const {
  createContinueState,
  observeContinueState,
  shouldAutoContinue,
  AUTO_CONTINUE_MAX_ROUNDS,
  prepareContinueStateForNextRound,
} = require('./stream_continue');
const { sendFailedChunk, upstreamEmptyOutputDetail } = require('./stream_failure');
const { detectMutedOrBannedResponse } = require('./upstream_inspect');

const DEEPSEEK_COMPLETION_URL = 'https://chat.deepseek.com/api/v0/chat/completion';
const DEEPSEEK_CONTINUE_URL = 'https://chat.deepseek.com/api/v0/chat/continue';

async function handleVercelStream(req, res, rawBody, payload, appFetch = fetchAppStream) {
  let leaseCompletion = {};
  const lifecycle = createStreamLifecycle(req, res, () => leaseCompletion);
  const releaseLease = lifecycle.release;
  try {
    if (lifecycle.closed) return;
    const prep = await fetchStreamPrepare(req, rawBody);
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
    const finalPrompt = asString(prep.body.final_prompt);
    const thinkingEnabled = toBool(prep.body.thinking_enabled);
    const searchEnabled = toBool(prep.body.search_enabled);
    const toolPolicy = resolveToolcallPolicy(prep.body, payload.tools);
    const toolNames = toolPolicy.toolNames;
    const emitEarlyToolDeltas = toolPolicy.emitEarlyToolDeltas;
    const stripReferenceMarkers = true;

    if (!model || !leaseID || !deepseekToken || !initialPowHeader || !completionPayload) {
      writeOpenAIError(res, 500, 'invalid vercel prepare response');
      return;
    }

    let baseHeaders = prep.body.base_headers && typeof prep.body.base_headers === 'object'
      ? { ...BASE_HEADERS, ...prep.body.base_headers }
      : { ...BASE_HEADERS };
    let headerExpirations = prep.body.base_header_expirations || {};
    const updateBaseHeaders = (switchedBody) => {
      if (switchedBody && switchedBody.base_headers && typeof switchedBody.base_headers === 'object') {
        baseHeaders = { ...BASE_HEADERS, ...switchedBody.base_headers };
        headerExpirations = switchedBody.base_header_expirations || {};
      }
    };

    let currentPowHeader = initialPowHeader;
    const refreshPowHeader = async (roundType) => {
      if (lifecycle.closed) return '';
      try {
        const pow = await fetchStreamPow(req, leaseID);
        if (lifecycle.closed) return '';
        const nextPowHeader = asString(pow.body && pow.body.pow_header);
        if (pow.ok && nextPowHeader) {
          currentPowHeader = nextPowHeader;
          return currentPowHeader;
        }
        console.warn('[vercel_stream_pow] refresh failed, reusing previous PoW', {
          round_type: roundType,
          status: pow.status || 0,
        });
      } catch (err) {
        if (lifecycle.closed || isAbortError(err)) {
          return '';
        }
        console.warn('[vercel_stream_pow] refresh failed, reusing previous PoW', {
          round_type: roundType,
          error: err,
        });
      }
      return currentPowHeader;
    };

    const fetchDeepSeekStream = async (url, bodyPayload, powHeader) => {
      if (lifecycle.closed) return null;
      try {
        const response = await appFetch(url, {
          method: 'POST',
          headerExpirations,
          headers: {
            ...baseHeaders,
            authorization: `Bearer ${deepseekToken}`,
            ...(powHeader ? { 'x-ds-pow-response': powHeader } : {}),
          },
          body: serializeAppPayload(url, bodyPayload),
          signal: lifecycle.signal,
        });
        if (lifecycle.closed) {
          try { await response.body?.cancel(); }
          catch (error) { console.warn('[vercel_stream] late response cancel failed', error.message); }
          return null;
        }
        return response;
      } catch (err) {
        if (lifecycle.closed || isAbortError(err)) {
          return null;
        }
        throw err;
      }
    };
    const fetchCompletion = (bodyPayload) => fetchDeepSeekStream(DEEPSEEK_COMPLETION_URL, bodyPayload, currentPowHeader);
    let activeDeepSeekSessionID = responseID;
    const fetchContinue = async (messageID) => {
      return fetchDeepSeekStream(DEEPSEEK_CONTINUE_URL, {
        chat_session_id: activeDeepSeekSessionID,
        message_id: messageID,
        fallback_to_resume: true,
      }, '');
    };
    // applySwitchedPayload 把 __stream_switch 返回的新账号 payload / token / PoW
    // / session 应用到当前流式请求，供首包与重试阶段的临时上游故障切号复用。
    const applySwitchedPayload = (switched) => {
      completionPayload = switched.body.payload;
      deepseekToken = asString(switched.body.deepseek_token) || deepseekToken;
      currentPowHeader = asString(switched.body.pow_header) || currentPowHeader;
      activeDeepSeekSessionID = asString(switched.body.session_id) || activeDeepSeekSessionID;
      updateBaseHeaders(switched.body);
    };

    // Let an in-flight Go control operation settle before releasing its lease,
    // but do not start a new one or replay completion after disconnect.
    const switchAccount = async (options) => {
      if (lifecycle.closed) return null;
      const switched = await fetchStreamSwitch(req, leaseID, options);
      return lifecycle.closed ? null : switched;
    };

    let completionRes = await fetchCompletion(completionPayload);
    if (completionRes === null) {
      return;
    }
    if (lifecycle.closed) {
      return;
    }

    // 当 DeepSeek 返回封号/停用 JSON（HTTP 200 + JSON body）时，持久化状态并
    // 切换账号。循环切换直到拿到健康的 SSE 流，或没有更多可调度账号；每次切换
    // 都会让 Go 侧持久化状态并触发弹性号池补位，因此全程无需人工干预。
    let accountSwitchCount = 0;
    while (true) {
      if (lifecycle.closed) return;
      const detection = await detectMutedOrBannedResponse(completionRes);
      if (lifecycle.closed) return;
      if (detection.response) {
        completionRes = detection.response;
        break;
      }
      if (!detection.banned && !(detection.mutedUntil > 0)) {
        // JSON body 但既非封号也非禁言：无可切换依据，交给下游按空输出处理。
        break;
      }
      accountSwitchCount += 1;
      const switchOptions = detection.banned
        ? { banned: true, banned_reason: '账户已被停用' }
        : { mute_until: detection.mutedUntil };
      const switched = await switchAccount(switchOptions);
      if (!switched) return;
      if (!(switched.ok && switched.body && switched.body.payload && typeof switched.body.payload === 'object')) {
        await releaseLease();
        if (detection.banned) {
          writeOpenAIErrorWithCode(res, 403, 'Account is banned by upstream.', 'account_banned');
        } else {
          writeOpenAIErrorWithCode(res, 403, 'Account is muted by upstream.', 'account_muted');
        }
        return;
      }
      console.warn('[vercel_stream_account_switch] retrying on alternate account after upstream mute/ban', {
        switch_count: accountSwitchCount,
        reason: detection.banned ? 'banned' : 'muted',
      });
      applySwitchedPayload(switched);
      completionRes = await fetchCompletion(completionPayload);
      if (completionRes === null) {
        return;
      }
      if (lifecycle.closed) {
        return;
      }
    }

    if (!completionRes.ok || !completionRes.body) {
      let status = completionRes.ok ? 500 : completionRes.status || 500;
      let detail = completionRes.body ? await completionRes.text() : '';
      // 临时上游故障（500/502/503/504）与账号级鉴权失败（401）都先尝试切号重放，
      // 而不是把上游报错原样透传给客户端；与 Go 侧 StartCompletion 的临时错误
      // 升级、retryManagedAuthFailure 的刷新 Token 后换号保持一致。
      let switchAttempts = 0;
      while (
        (isTransientUpstreamStatus(status) || isAccountAuthFailureStatus(status))
        && switchAttempts < TRANSIENT_SWITCH_MAX_ATTEMPTS
      ) {
        const switched = await switchAccount(switchOptionsForStatus(status));
        if (!switched) return;
        if (!(switched.ok && switched.body && switched.body.payload && typeof switched.body.payload === 'object')) {
          break;
        }
        switchAttempts += 1;
        console.warn('[vercel_stream_account_switch] retrying after upstream status', {
          status,
          switch_count: switchAttempts,
        });
        applySwitchedPayload(switched);
        completionRes = await fetchCompletion(completionPayload);
        if (completionRes === null) {
          return;
        }
        if (completionRes.ok && completionRes.body) {
          break;
        }
        status = completionRes.ok ? 500 : completionRes.status || 500;
        detail = completionRes.body ? await completionRes.text() : '';
      }
      if (!completionRes.ok || !completionRes.body) {
        writeOpenAIError(res, status, detail);
        return;
      }
    }

    if (lifecycle.closed) return;
    res.statusCode = 200;
    res.setHeader('Content-Type', 'text/event-stream');
    res.setHeader('Cache-Control', 'no-cache, no-transform');
    res.setHeader('Connection', 'keep-alive');
    res.setHeader('X-Accel-Buffering', 'no');
    if (typeof res.flushHeaders === 'function') {
      res.flushHeaders();
    }

    const created = Math.floor(Date.now() / 1000);
    let currentType = thinkingEnabled ? 'thinking' : 'text';
    let thinkingText = '';
    // Monotonic "has any non-whitespace char" flags: replaces per-line
    // outputText.trim() scans, which are O(total) per SSE line (O(n^2) per stream).
    let hasVisibleThinking = false;
    let detectionThinkingText = '';
    let outputText = '';
    let hasVisibleOutput = false;
    let deferredToolText = '';
    // 上游正文通道的内容规则（复读死循环 / 编造的 `[Tool]:` 角色标记）统一由
    // 该策略持有，与 Go 侧 sse.ContentPolicy 语义一致；新增规则只需改策略本身。
    // 每个上游轮次在 processStream 的轮循环顶部重建一次（见下），因为规则状态
    // 属于一条上游流：上一轮命中的截断不能继续吞掉这一轮的正文。
    let contentPolicy = createContentPolicy();
    let usagePrompt = finalPrompt;
    const toolSieveEnabled = toolPolicy.toolSieveEnabled;
    let toolSieveState = createToolSieveState();
    toolSieveState.deferMalformed = true;
    // 调用者专属工具调用标识（由 Go 侧按 API Key 派生并随 prepare 下发）。
    // 上游只会输出该标识，而 sieve / 独立解析 / 复读守卫都只认 EPSE，
    // 因此所有流入文本都要先经它有状态地归一化回 EPSE（跨分片标识会被
    // 暂存到下一片，避免半截标识泄漏成正文）。
    let markerNormalizer = createMarkerNormalizer(prep.body.tool_marker);
    let toolCallsEmitted = false;
    let toolCallsDoneEmitted = false;
    let toolCallOffset = 0;
    const streamToolCallIDs = new Map();
    const streamToolNames = new Map();
    const decoder = new TextDecoder();
    let buffered = '';
    let ended = false;
    let lastEmptyDetail = null;
    let answerDelivered = false;
    const { sendFrame, sendDeltaFrame: emitDeltaFrame } = createChatCompletionEmitter({
      res,
      sessionID: responseID,
      created,
      model,
      isClosed: () => lifecycle.closed,
    });
    const sendDeltaFrame = (delta) => {
      if (delta.content || delta.tool_calls) answerDelivered = true;
      emitDeltaFrame(delta);
    };
    const deltaCoalescer = createDeltaCoalescer({ sendDeltaFrame });
    const resetForRegeneration = () => {
      outputText = '';
      hasVisibleOutput = false;
      deferredToolText = '';
      detectionThinkingText = '';
      lastEmptyDetail = null;
      toolSieveState = createToolSieveState();
      toolSieveState.deferMalformed = true;
      markerNormalizer = createMarkerNormalizer(prep.body.tool_marker);
      resetStreamToolCallState(streamToolCallIDs, streamToolNames);
    };

    const finish = async (reason, options = {}) => {
      if (ended) {
        return true;
      }
      if (lifecycle.closed || res.writableEnded || res.destroyed) {
        ended = true;
        leaseCompletion = {
          finish_reason: 'stopped',
          usage: buildUsage(usagePrompt, thinkingText, outputText),
          thinking: thinkingText,
          content: outputText,
          error: '',
        };
        await releaseLease();
        return true;
      }
      deltaCoalescer.flush();
      // 收尾前释放归一化器暂存的标识残片，并并入累计文本，使 finish 阶段的
      // 独立工具调用解析与逐片 sieve 看到同一份 EPSE 形态文本。残片只可能是
      // 调用方标识（字母表不含 `[`）的前缀，因此只喂复读守卫、不喂角色标记
      // 守卫——后者会把残片与它自己暂存的标记前缀合并后一起放行，把本该丢弃的
      // `[To` 泄漏成正文（见 feedContentPolicyTail）。
      const markerTail = markerNormalizer.flush();
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
      let validatedCalls = null;
      if (prep.body.validate_tools === true && toolPolicy.constrained && (hasVisibleOutput || detected.length)) {
        try {
          const validation = await fetchStreamValidation(req, leaseID, outputText, thinkingText, detectionThinkingText, lifecycle.signal);
          if (!validation.ok) {
            const error = validation.body.error || {};
            if (options.deferEmpty && !answerDelivered && ['invalid_tool_arguments', 'tool_choice_violation'].includes(error.code)) {
              lastEmptyDetail = { status: validation.status, message: error.message || 'Tool validation failed', code: error.code };
              return false;
            }
            ended = true;
            leaseCompletion = { finish_reason: 'error', usage: null, thinking: thinkingText, content: outputText, error: error.message || 'Tool validation failed' };
            sendFailedChunk(res, validation.status, leaseCompletion.error, error.code || 'invalid_tool_arguments');
            await releaseLease(); if (!res.writableEnded && !res.destroyed) res.end(); return true;
          }
          validatedCalls = validation.body.tool_calls || [];
          detected = validatedCalls.map(call => ({ name: call.function.name }));
        } catch (error) {
          if (lifecycle.closed) {
            ended = true; leaseCompletion = { finish_reason: 'stopped', usage: null, thinking: thinkingText, content: outputText, error: '' };
            await releaseLease(); return true;
          }
          ended = true;
          leaseCompletion = { finish_reason: 'error', usage: null, thinking: thinkingText, content: outputText, error: 'Tool validation unavailable: ' + error.message };
          sendFailedChunk(res, 502, leaseCompletion.error, 'tool_validation_unavailable');
          await releaseLease(); if (!res.writableEnded && !res.destroyed) res.end(); return true;
        }
      }
      // An empty upstream result must retain its retry/error path. Required
      // tool policy applies to actual answers, not a missing response.
      const violation = (detected.length > 0 || toolCallsEmitted || hasVisibleOutput)
        ? toolChoiceViolation(detected, toolPolicy.choice) : '';
      if (violation) {
        if (options.deferEmpty && !answerDelivered) {
          lastEmptyDetail = { status: 422, message: violation, code: 'tool_choice_violation' };
          return false;
        }
        ended = true;
        leaseCompletion = { finish_reason: 'error', usage: null, thinking: thinkingText, content: '', error: violation };
        sendFailedChunk(res, 422, violation, 'tool_choice_violation');
        await releaseLease();
        if (!res.writableEnded && !res.destroyed) res.end();
        return true;
      }
      if (!detected.length && !toolCallsEmitted && deferredToolText) { deltaCoalescer.append('content', deferredToolText); deferredToolText = ''; }
      if (detected.length > 0 && !toolCallsDoneEmitted) {
        toolCallsEmitted = true;
        toolCallsDoneEmitted = true;
        sendDeltaFrame({ tool_calls: validatedCalls || formatOpenAIStreamToolCalls(detected, streamToolCallIDs, payload.tools) });
      } else if (toolSieveEnabled) {
        const tailEvents = markerTail
          ? processToolSieveChunk(toolSieveState, markerTail, toolNames).concat(flushToolSieve(toolSieveState, toolNames))
          : flushToolSieve(toolSieveState, toolNames);
        for (const evt of tailEvents) {
          if (evt.type === 'tool_calls' && Array.isArray(evt.calls) && evt.calls.length > 0) {
            if (toolPolicy.constrained) continue;
            deltaCoalescer.flush();
            toolCallsEmitted = true;
            toolCallsDoneEmitted = true;
            sendDeltaFrame({ tool_calls: formatOpenAIStreamToolCalls(evt.calls, streamToolCallIDs, payload.tools, toolCallOffset) });
            toolCallOffset += streamToolCallIDs.size;
            resetStreamToolCallState(streamToolCallIDs, streamToolNames);
            continue;
          }
          if (evt.text) {
            // 已发出 tool_calls 后，收尾阶段剩余的工具块后正文同样丢弃，与流式处理一致。
            if (toolCallsEmitted) {
              continue;
            }
            deltaCoalescer.append('content', evt.text);
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
        leaseCompletion = { finish_reason: reason, usage: null, thinking: thinkingText, content: outputText, error: detail.message };
        sendFailedChunk(res, detail.status, detail.message, detail.code);
        await releaseLease();
        if (!res.writableEnded && !res.destroyed) {
          res.end();
        }
        return true;
      }
      ended = true;
      const finalUsage = buildUsage(usagePrompt, thinkingText, outputText);
      sendFrame({
        id: responseID,
        object: 'chat.completion.chunk',
        created,
        model,
        choices: [{ delta: {}, index: 0, finish_reason: reason }],
        usage: finalUsage,
      });
      if (!res.writableEnded && !res.destroyed) {
        res.write('data: [DONE]\n\n');
      }
      leaseCompletion = {
        finish_reason: reason,
        usage: finalUsage,
        thinking: thinkingText,
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
      // eslint-disable-next-line no-constant-condition
      while (true) {
        // 每一轮都是一次新的上游响应——自动续答轮、换号重试轮都从这里进入——
        // 因此内容规则重新开始：上一轮命中的截断只属于上一轮，若继续生效，
        // 这一轮的健康正文会被静默丢弃，客户端最后只会拿到
        // upstream_unavailable。与 Go 侧每个 attempt 重置一次对齐
        // （consume*StreamAttempt → ResetContentPolicy）。
        contentPolicy = createContentPolicy();
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
            const { value, done } = await lifecycle.reader.read();
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
              const parsed = parseChunkForContent(chunk, thinkingEnabled, currentType, stripReferenceMarkers);
              if (!parsed.parsed) {
                continue;
              }
              currentType = parsed.newType;
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
                leaseCompletion = { finish_reason: 'error', usage: null, thinking: thinkingText, content: outputText, error: parsed.errorMessage };
                sendFailedChunk(res, 502, parsed.errorMessage, 'upstream_error');
                await releaseLease();
                if (!res.writableEnded && !res.destroyed) res.end();
                return { terminal: true, retryable: false };
              }
              if (parsed.contentFilter) {
                return { terminal: await finish(!hasVisibleOutput ? 'content_filter' : 'stop'), retryable: false };
              }
              if (parsed.finished) {
                streamEnded = true;
                break;
              }

              if (parsed.detectionThinking && !hasVisibleOutput) {
                detectionThinkingText += trimContinuationOverlap(detectionThinkingText, parsed.detectionThinking);
              }
              for (const p of parsed.parts) {
                if (!p.text) {
                  continue;
                }
                if (p.type === 'thinking') {
                  if (thinkingEnabled) {
                    const trimmed = trimContinuationOverlap(thinkingText, p.text);
                    if (!trimmed) {
                      continue;
                    }
                    thinkingText += trimmed;
                    if (!hasVisibleThinking) {
                      hasVisibleThinking = trimmed.trim() !== '';
                    }
                    deltaCoalescer.append('reasoning_content', trimmed);
                  }
                } else {
                  // 先归一化再进入复读守卫 / 累计文本 / sieve：三者都只认 EPSE。
                  // thinking 通道刻意不做归一化，与 Go 侧
                  // chatStreamRuntime.onParsed 保持一致（Go 只在 assistantturn
                  // 收口时归一化 thinking，流式 reasoning_content 片段原样下发）。
                  const normalizedText = markerNormalizer.write(p.text);
                  if (!normalizedText) {
                    continue;
                  }
                  const trimmed = trimContinuationOverlap(outputText, normalizedText);
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
                  if (searchEnabled && isCitation(safeText)) {
                    continue;
                  }
                  outputText += safeText;
                  if (!hasVisibleOutput) {
                    hasVisibleOutput = safeText.trim() !== '';
                  }
                  if (!toolSieveEnabled) {
                    deltaCoalescer.append('content', safeText);
                    continue;
                  }
                  const events = processToolSieveChunk(toolSieveState, safeText, toolNames);
                  for (const evt of events) {
                    if (evt.deferred) { deferredToolText += evt.text || ''; continue; }
                    if (evt.type === 'tool_call_deltas') {
                      if (!emitEarlyToolDeltas) {
                        continue;
                      }
                      const filtered = filterIncrementalToolCallDeltasByAllowed(evt.deltas, toolNames, streamToolNames);
                      const formatted = formatIncrementalToolCallDeltas(filtered, streamToolCallIDs, toolCallOffset);
                      if (formatted.length > 0) {
                        toolCallsEmitted = true;
                        deltaCoalescer.flush();
                        sendDeltaFrame({ tool_calls: formatted });
                      }
                      continue;
                    }
                    if (evt.type === 'tool_calls') {
                      if (toolPolicy.constrained) continue;
                      toolCallsEmitted = true;
                      toolCallsDoneEmitted = true;
                      deltaCoalescer.flush();
                      sendDeltaFrame({ tool_calls: formatOpenAIStreamToolCalls(evt.calls, streamToolCallIDs, payload.tools, toolCallOffset) });
                      toolCallOffset += streamToolCallIDs.size;
                      resetStreamToolCallState(streamToolCallIDs, streamToolNames);
                      continue;
                    }
                    if (evt.text) {
                      // 已发出 tool_calls 后，工具调用块之后追加的尾巴正文不再透传
                      // （OpenAI 规范中 tool_calls 回合的 content 应为空）。
                      if (toolCallsEmitted) {
                        continue;
                      }
                      deltaCoalescer.append('content', evt.text);
                    }
                  }
                }
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
          if (lifecycle.closed || isAbortError(err)) {
            await finish('stop');
            return { terminal: true, retryable: false, interrupted: false };
          }
          // Upstream stream interrupted mid-body (RST / EOF without terminal
          // status). If the response already produced visible content and the
          // upstream had not signalled completion with [DONE], try to resume
          // the same message via the continue endpoint instead of silently
          // delivering a truncated response.
          if ((hasVisibleOutput || hasVisibleThinking) && !continueState.sawDone && shouldAutoContinue(continueState) && continueRounds < AUTO_CONTINUE_MAX_ROUNDS) {
            continueRounds += 1;
            const nextRes = await fetchContinue(continueState.responseMessageID);
            if (nextRes !== null && nextRes.ok && nextRes.body) {
              continueState = prepareContinueStateForNextRound(continueState);
              currentResponse = nextRes;
              continue;
            }
          }
          if (hasVisibleOutput || hasVisibleThinking) {
            return { terminal: false, retryable: false, interrupted: true, responseMessageID: continueState.responseMessageID };
          }
          // No partial output: fall through to the normal empty-output path.
          break;
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
          if (nextRes === null) {
            return { terminal: true, retryable: false };
          }
          if (!nextRes.ok || !nextRes.body) {
            return { terminal: await finish('stop'), retryable: false };
          }
          continueState = prepareContinueStateForNextRound(continueState);
          currentResponse = nextRes;
          continue;
        }
        break;
      }

      const terminal = await finish('stop', { deferEmpty: allowDeferEmpty });
      return { terminal, retryable: !terminal && allowDeferEmpty, responseMessageID: continueState.responseMessageID, interrupted: false };
    };

    let retryAttempts = 0;
    let accountSwitchAttempted = false;
    let transientSwitchAttempts = 0;
    // eslint-disable-next-line no-constant-condition
    while (true) {
      if (!completionRes.ok || !completionRes.body) {
        const status = completionRes.ok ? 502 : completionRes.status || 502;
        const message = completionRes.body && !completionRes.bodyUsed ? await completionRes.text() : (lastEmptyDetail && lastEmptyDetail.message) || '';
        if (!isTransientUpstreamStatus(status) && !isAccountAuthFailureStatus(status) && status !== 429) {
          ended = true;
          leaseCompletion = { finish_reason: 'error', usage: null, thinking: thinkingText, content: outputText, error: message };
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
      const isAccountUnauthorized = lastEmptyDetail && lastEmptyDetail.code === 'account_unauthorized';
      const isInputContentBlocked = lastEmptyDetail && lastEmptyDetail.code === 'input_content_blocked';
      const isToolOutputError = lastEmptyDetail && ['invalid_tool_arguments', 'tool_choice_violation'].includes(lastEmptyDetail.code);
      if (processed.terminal) {
        return;
      }
      if (processed.interrupted) {
        // The upstream stream was cut after partial output and the in-stream
        // continue resume did not recover it: surface the interruption instead
        // of finishing with a truncated "complete" response.
        deltaCoalescer.flush();
        ended = true;
        leaseCompletion = { finish_reason: 'upstream_interrupted', usage: null, thinking: thinkingText, content: outputText, error: 'Upstream stream interrupted before completion.' };
        await releaseLease();
        sendFailedChunk(res, 502, 'Upstream stream interrupted before completion.', 'upstream_interrupted');
        if (!res.writableEnded && !res.destroyed) res.end();
        return;
      }
      if (!processed.retryable) {
        await finish('stop');
        return;
      }
      if (retryAttempts >= EMPTY_OUTPUT_RETRY_MAX_ATTEMPTS) {
        if (isInputContentBlocked) {
          await finish('stop');
          return;
        }
        if (isUpstreamUnavailable || isAccountUnauthorized) {
          if (answerDelivered || transientSwitchAttempts >= TRANSIENT_SWITCH_MAX_ATTEMPTS) {
            await finish('stop');
            return;
          }
          transientSwitchAttempts += 1;
          const switched = await switchAccount(isAccountUnauthorized ? { disable: true, auth_failed: true } : { disable: true });
          if (!switched) return;
          if (switched.ok && switched.body && switched.body.payload && typeof switched.body.payload === 'object') {
            applySwitchedPayload(switched);
            usagePrompt = finalPrompt;
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
            usagePrompt = finalPrompt;
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
      usagePrompt = isToolOutputError
        ? `${finalPrompt}\n${asString(completionPayload.prompt)}\n\n${TOOL_OUTPUT_RETRY_SUFFIX}`
        : usagePromptWithEmptyOutputRetry(finalPrompt, retryAttempts);
      const retryPowHeader = await refreshPowHeader('retry');
      if (!retryPowHeader) {
        return;
      }
      completionRes = await fetchDeepSeekStream(
        DEEPSEEK_COMPLETION_URL,
        isToolOutputError
          ? { ...clonePayloadForEmptyOutputRetry(completionPayload, processed.responseMessageID), prompt: `${asString(completionPayload.prompt)}\n\n${TOOL_OUTPUT_RETRY_SUFFIX}` }
          : clonePayloadForEmptyOutputRetry(completionPayload, processed.responseMessageID),
        retryPowHeader,
      );
      if (completionRes === null) {
        return;
      }
      if (!completionRes.ok || !completionRes.body) {
        const retryStatus = completionRes.ok ? 500 : completionRes.status || 500;
        if (isTransientUpstreamStatus(retryStatus) || isAccountAuthFailureStatus(retryStatus)) {
          const retryDetail = completionRes.body ? await completionRes.text() : '';
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
  } finally {
    await lifecycle.dispose();
  }
}

function toBool(v) {
  return v === true;
}

module.exports = {
  handleVercelStream,
};
