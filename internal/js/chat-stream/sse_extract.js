'use strict';

// Extract nested fragment patches without exposing protocol metadata.
const { asString, asContentString } = require('./sse_text');
const { isStatusPath, isFragmentScopedPath, isFinishedStatus, shouldSkipPath, isToolFragmentType } = require('./sse_paths');

function extractContentRecursive(items, defaultType, newType, stripReferenceMarkers = true, thinkingEnabled = true, parentPath = '') {
  const parts = [];
  for (const it of items) {
    if (!it || typeof it !== 'object') {
      continue;
    }
    if (!Object.prototype.hasOwnProperty.call(it, 'v')) {
      continue;
    }
    const itemPath = asString(it.p);
    const itemV = it.v;
    // Status ops nested under a fragment-scoped path
    // (e.g. {"p":"response/fragments/-1","v":[{"p":"status","v":"FINISHED"}]})
    // mark that fragment as done, not the whole response. Treating them as
    // terminal cut streams short right after a web-search fragment such as
    // "正在搜索图片".
    if (isStatusPath(itemPath) && !isFragmentScopedPath(parentPath)) {
      if (isFinishedStatus(itemV)) {
        return { parts: [], finished: true, newType };
      }
      continue;
    }
    if (shouldSkipPath(itemPath)) {
      continue;
    }
    const content = asContentString(it.content, stripReferenceMarkers);
    if (content) {
      const typeName = asString(it.type).toUpperCase();
      if (typeName === 'THINK' || typeName === 'THINKING') {
        // THINK fragment moves the sticky cursor to thinking; RESPONSE (or
        // response/content) is the only way back to text. Once the body has
        // begun (one-size-fits-all rule) the cursor never rewinds.
        if (!(thinkingEnabled && newType === 'text')) {
          newType = 'thinking';
        }
        parts.push({ text: content, type: 'thinking' });
      } else if (typeName === 'RESPONSE') {
        newType = 'text';
        parts.push({ text: content, type: 'text' });
      } else if (isToolFragmentType(typeName)) {
        parts.push({ text: content, type: 'thinking' });
      } else {
        parts.push({ text: content, type: defaultType });
      }
      continue;
    }

    let partType = defaultType;
    if (isFragmentScopedPath(parentPath) && (itemPath === 'content' || itemPath === 'response/content')) {
      // A bare response/fragments/-N patch (no /content suffix) updates the
      // fragment object itself. In the web-search flow these are the tool
      // fragment's status/content (e.g. "已搜索到相关图片"), which is progress
      // narration rather than answer body.
      partType = 'thinking';
    } else if (itemPath.includes('thinking')) {
      partType = 'thinking';
    } else if (itemPath === 'content' || itemPath === 'response' || itemPath === 'fragments' || itemPath === 'response/content') {
      // Explicit body/fragment paths stay text. DeepSeek also emits path
      // variants like "fragments/-1/content" (no "response/" prefix) inside a
      // response BATCH; those carry the CURRENT stream kind (thinking content
      // while reasoning, body content while text), so they follow the sticky
      // cursor instead of being forced to text.
      partType = 'text';
    } else if (newType) {
      partType = newType;
    }

    if (typeof itemV === 'string') {
      if (isStatusPath(itemPath)) {
        continue;
      }
      if (itemV && itemV !== 'FINISHED') {
        const content = asContentString(itemV, stripReferenceMarkers);
        if (content) {
          parts.push({ text: content, type: partType });
        }
      }
      continue;
    }

    if (!Array.isArray(itemV)) {
      continue;
    }
    for (const inner of itemV) {
      if (typeof inner === 'string') {
        if (inner) {
          const content = asContentString(inner, stripReferenceMarkers);
          if (content) {
            parts.push({ text: content, type: partType });
          }
        }
        continue;
      }
      if (!inner || typeof inner !== 'object') {
        continue;
      }
      if (shouldSkipPath(asString(inner.p))) {
        continue;
      }
      const ct = asContentString(inner.content, stripReferenceMarkers);
      if (!ct) {
        // Nested fragment items may carry the payload as "v" with no "content"
        // field, e.g. a fragment content APPEND inside a fragments BATCH:
        //   {"p":"-1/content","o":"APPEND","v":"..."}
        // Use the sticky cursor for its type so body code (e.g. a trailing
        // "pass\n```") is not dropped and reasoning fragments are not leaked
        // as body text. TIP-style items still fall through when "v" is not a
        // string.
        const sv = typeof inner.v === 'string' ? asContentString(inner.v, stripReferenceMarkers) : '';
        if (sv) {
          parts.push({ text: sv, type: newType || partType });
        }
        continue;
      }
      const typeName = asString(inner.type).toUpperCase();
      if (typeName === 'THINK' || typeName === 'THINKING') {
        // THINK fragment moves the sticky cursor to thinking; RESPONSE (or
        // response/content) is the only way back to text. Once the body has
        // begun (one-size-fits-all rule) the cursor never rewinds.
        if (!(thinkingEnabled && newType === 'text')) {
          newType = 'thinking';
        }
        parts.push({ text: ct, type: 'thinking' });
      } else if (typeName === 'RESPONSE') {
        newType = 'text';
        parts.push({ text: ct, type: 'text' });
      } else if (isToolFragmentType(typeName)) {
        parts.push({ text: ct, type: 'thinking' });
      } else {
        parts.push({ text: ct, type: partType });
      }
    }
  }
  return { parts, finished: false, newType };
}

module.exports = {
  extractContentRecursive,
};
