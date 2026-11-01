'use strict';

// Decode one upstream content chunk; public helpers are re-exported here.
const { asString, asContentString, stripReferenceMarkersText, stripThinkTags } = require('./sse_text');
const { hasContentFilterStatus } = require('./sse_content_filter');
const { shouldSkipPath, isStatusPath, isFinishedStatus, isToolFragmentType, isFragmentStatusPath, isCitation } = require('./sse_paths');
const { finalizeThinkingParts } = require('./sse_thinking');
const { isContentSnapshot } = require('./sse_snapshot');
const { SnapshotDecoder, coalesceSnapshotParts } = require('./sse_replay');
const { extractContentRecursive } = require('./sse_extract');

function parseChunkForContent(chunk, thinkingEnabled, currentType, stripReferenceMarkers = true, state = {}) {
  // Decode losslessly. Reference/control cleanup belongs to the display sieve,
  // after tool parameters have been separated from visible text.
  stripReferenceMarkers = false;
  if (!chunk || typeof chunk !== 'object') {
    return { parsed: false, parts: [], finished: false, contentFilter: false, errorMessage: '', outputTokens: 0, newType: currentType };
  }
  const usage = extractAccumulatedTokenUsage(chunk);
  const result = {
    parsed: true, parts: [], finished: false, contentFilter: false,
    errorMessage: '', promptTokens: usage.prompt, outputTokens: usage.output, newType: currentType, snapshot: isContentSnapshot(chunk),
  };
  if (Object.prototype.hasOwnProperty.call(chunk, 'error')) {
    return { ...result, finished: true, errorMessage: formatErrorMessage(chunk.error) };
  }
  const pathValue = asString(chunk.p);
  if (hasContentFilterStatus(chunk)) return { ...result, finished: true, contentFilter: true };
  if (isStatusPath(pathValue)) return { ...result, finished: isFinishedStatus(chunk.v) };
  if (shouldSkipPath(pathValue)) return result;
  if (!Object.prototype.hasOwnProperty.call(chunk, 'v')) return result;

  state.snapshotDecoder ||= new SnapshotDecoder();
  chunk = state.snapshotDecoder.normalize(chunk, currentType, thinkingEnabled);
  result.snapshot = isContentSnapshot(chunk);
  if (state.snapshotDecoder.error) return { ...result, finished: true, errorMessage: state.snapshotDecoder.error };

  let newType = currentType;
  const parts = [];
  let finished = false;
  const bodyStarted = thinkingEnabled && currentType === 'text';
  if (pathValue === 'response/fragments' && asString(chunk.o).toUpperCase() === 'APPEND' && Array.isArray(chunk.v)) {
    for (const frag of chunk.v) {
      if (!frag || typeof frag !== 'object') {
        continue;
      }
      const fragType = asString(frag.type).toUpperCase();
      const content = asContentString(frag.content, stripReferenceMarkers);
      if (!content) {
        continue;
      }
      if (fragType === 'THINK' || fragType === 'THINKING') {
        if (!(thinkingEnabled && newType === 'text')) {
          newType = 'thinking';
        }
        parts.push({ text: content, type: 'thinking' });
      } else if (fragType === 'RESPONSE') {
        newType = 'text';
        parts.push({ text: content, type: 'text' });
      } else if (isToolFragmentType(fragType)) {
        parts.push({ text: content, type: 'thinking' });
      } else {
        parts.push({ text: content, type: 'text' });
      }
    }
  }

  if (pathValue === 'response' && Array.isArray(chunk.v)) {
    for (const item of chunk.v) {
      if (!item || typeof item !== 'object') {
        continue;
      }
      if (item.p === 'fragments' && item.o === 'APPEND' && Array.isArray(item.v)) {
        for (const frag of item.v) {
          const fragType = asString(frag && frag.type).toUpperCase();
          if (fragType === 'THINK' || fragType === 'THINKING') {
            if (!(thinkingEnabled && newType === 'text')) {
              newType = 'thinking';
            }
          } else if (fragType === 'RESPONSE') {
            newType = 'text';
          }
        }
      }
    }
  }

  if (pathValue === 'response/content') {
    newType = 'text';
  } else if (pathValue === 'response/thinking_content' && (!thinkingEnabled || newType !== 'text')) {
    newType = 'thinking';
  }

  let partType = 'text';
  if (pathValue === 'response/thinking_content') {
    // Reasoning content is always typed "thinking". When the body has already
    // begun, finalizeThinkingParts drops it wholesale (one-size-fits-all rule)
    // instead of the old behavior that recovered it as body text and leaked
    // reasoning into the visible answer.
    partType = 'thinking';
  } else if (pathValue === 'response/content') {
    partType = 'text';
  } else if ((pathValue.startsWith('fragments/') || pathValue.startsWith('response/fragments/')) && pathValue.includes('/content')) {
    // Fragment content paths follow the sticky cursor. DeepSeek emits the
    // relative form "fragments/-1/content" (no "response/" prefix) inside a
    // response BATCH; it carries the current stream kind, not a body marker.
    partType = newType;
  } else if (!pathValue) {
    partType = newType || 'text';
  }


  const val = chunk.v;
  if (typeof val === 'string') {
    if (isFinishedStatus(val) && (!pathValue || pathValue === 'status')) {
      return { ...result, finished: true, newType };
    }
    const content = asContentString(val, stripReferenceMarkers);
    if (content) parts.push({ text: content, type: partType });
  } else if (Array.isArray(val)) {
    const extracted = extractContentRecursive(val, partType, newType, stripReferenceMarkers, thinkingEnabled, pathValue);
    finished = extracted.finished;
    parts.push(...extracted.parts);
    newType = extracted.newType || newType;
  } else if (val && typeof val === 'object') {
    const directContent = asContentString(val, stripReferenceMarkers);
    if (directContent) {
      parts.push({ text: directContent, type: partType });
    }
    const resp = val.response && typeof val.response === 'object' ? val.response : val;
    finished = (!pathValue || pathValue === 'response') && (isFinishedStatus(resp.status) || isFinishedStatus(resp.quasi_status));
    if (Array.isArray(resp.fragments)) {
      for (const frag of resp.fragments) {
        if (!frag || typeof frag !== 'object') {
          continue;
        }
        const content = asContentString(frag.content, stripReferenceMarkers);
        if (!content) {
          continue;
        }
        const t = asString(frag.type).toUpperCase();
        if (t === 'THINK' || t === 'THINKING') {
          if (!(thinkingEnabled && newType === 'text')) {
            newType = 'thinking';
          }
          parts.push({ text: content, type: 'thinking' });
        } else if (t === 'RESPONSE') {
          newType = 'text';
          parts.push({ text: content, type: 'text' });
        } else if (isToolFragmentType(t)) {
          parts.push({ text: content, type: 'thinking' });
        } else {
          parts.push({ text: content, type: partType });
        }
      }
    }
  }
  
  const finalized = finalizeThinkingParts(result.snapshot ? coalesceSnapshotParts(parts) : parts, thinkingEnabled, newType, bodyStarted, state);
  return { ...result, finished, parts: finalized.parts, detectionThinking: finalized.detectionThinking || '', newType: finalized.newType };
}

function extractAccumulatedTokenUsage(chunk) {
  // 临时策略：忽略上游 usage 字段（accumulated_token_usage / token_usage），
  // 统一使用内部估算计数，避免上下文累计口径误差。
  void chunk;
  return { prompt: 0, output: 0 };
}

function formatErrorMessage(v) {
  if (typeof v === 'string') {
    return v;
  }
  if (v == null) {
    return String(v);
  }
  try {
    return JSON.stringify(v);
  } catch (_err) {
    return String(v);
  }
}

module.exports = {
  parseChunkForContent,
  extractContentRecursive,
  hasContentFilterStatus,
  extractAccumulatedTokenUsage,
  shouldSkipPath,
  isFragmentStatusPath,
  isCitation,
  stripReferenceMarkers: stripReferenceMarkersText,
  stripThinkTags,
};
