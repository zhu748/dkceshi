'use strict';

// Thinking/body transitions and the sticky body boundary.
const { stripThinkTags } = require('./sse_text');

function splitThinkingParts(parts) {
  const out = [];
  let thinkingDone = false;
  for (const p of parts) {
    if (!p) continue;
    if (thinkingDone && p.type === 'thinking') {
      const cleaned = stripThinkTags(p.text);
      if (cleaned) {
        out.push({ text: cleaned, type: 'text' });
      }
      continue;
    }
    if (p.type !== 'thinking') {
      const cleaned = stripThinkTags(p.text);
      if (cleaned) {
        out.push({ text: cleaned, type: p.type });
      }
      continue;
    }
    const match = /<\/\s*think\s*>/i.exec(p.text);
    if (!match) {
      out.push(p);
      continue;
    }
    thinkingDone = true;
    const before = p.text.substring(0, match.index);
    let after = p.text.substring(match.index + match[0].length);
    if (before) {
      out.push({ text: before, type: 'thinking' });
    }
    after = stripThinkTags(after);
    if (after) {
      out.push({ text: after, type: 'text' });
    }
  }
  return { parts: out, transitioned: thinkingDone };
}

function dropThinkingParts(parts) {
  if (!Array.isArray(parts) || parts.length === 0) {
    return parts;
  }
  return parts.filter((p) => p && p.type !== 'thinking');
}

function finalizeThinkingParts(parts, thinkingEnabled, newType, bodyStarted) {
  if (bodyStarted) {
    // 一刀切: visible body output has already begun for this request, so every
    // residual thinking/reasoning increment is dropped wholesale. It is neither
    // routed to thinking (no reasoning_content deltas) nor recovered into the
    // body via the </think> split (no thinking content folded into content).
    const out = [];
    for (const p of parts) {
      if (!p || p.type === 'thinking') {
        continue;
      }
      const cleaned = stripThinkTags(p.text);
      if (cleaned) {
        out.push({ ...p, text: cleaned });
      }
    }
    return { parts: out, newType };
  }
  const splitResult = splitThinkingParts(parts);
  let finalType = newType;
  let finalParts = splitResult.parts;
  if (splitResult.transitioned) {
    finalType = 'text';
  }
  if (!thinkingEnabled) {
    finalParts = dropThinkingParts(finalParts);
  }
  const detectionThinking = splitResult.parts.filter(p => p.type === 'thinking').map(p => p.text).join('');
  return { parts: finalParts, newType: finalType, detectionThinking };
}

module.exports = {
  finalizeThinkingParts,
};
