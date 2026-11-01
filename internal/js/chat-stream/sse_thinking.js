'use strict';

// Thinking/body transitions and the sticky body boundary.
const { LiteralState } = require('../helpers/literal-state');

const MAX_THINKING_DELIMITER = 256;

function closingPrefix(text) {
  if (text.length < 2) return [text === '<', false];
  if (!text.startsWith('</')) return [false, false];
  let rest = text.slice(2).replace(/^[ \t\r\n\f]+/, '');
  if (rest.length <= 5) return ['think'.startsWith(rest.toLowerCase()), false];
  if (rest.slice(0, 5).toLowerCase() !== 'think') return [false, false];
  rest = rest.slice(5).replace(/^[ \t\r\n\f]+/, '');
  return [rest === '' || rest === '>', rest === '>'];
}

function splitThinking(text, state) {
  const out = [];
  let offset = 0;
  for (const ch of text) {
    const structural = state.thinkingLiteral.feed(ch);
    offset += ch.length;
    if (state.thinkingPending) {
      const candidate = state.thinkingPending + ch;
      const [prefix, complete] = closingPrefix(candidate);
      if (complete) { state.thinkingPending = ''; return [out.join(''), text.slice(offset), true]; }
      if (prefix && candidate.length <= MAX_THINKING_DELIMITER) { state.thinkingPending = candidate; continue; }
      out.push(state.thinkingPending);
      state.thinkingPending = '';
    }
    if (structural && ch === '<') state.thinkingPending = '<';
    else out.push(ch);
  }
  return [out.join(''), '', false];
}

function flushThinkingState(state) {
  const text = state.thinkingPending || '';
  state.thinkingPending = '';
  return text;
}

function splitThinkingParts(parts, state) {
  const out = [];
  let thinkingDone = false;
  for (const p of parts) {
    if (!p) continue;
    if (thinkingDone && p.type === 'thinking') { if (p.text) out.push({ ...p, type: 'text' }); continue; }
    if (p.type !== 'thinking') {
      const pending = flushThinkingState(state);
      if (pending) out.push({ text: pending, type: 'thinking' });
      if (p.text) out.push(p);
      continue;
    }
    const [before, after, closed] = splitThinking(p.text, state);
    if (before) out.push({ ...p, text: before });
    if (closed) {
      thinkingDone = true;
      if (after) out.push({ ...p, text: after, type: 'text' });
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

function finalizeThinkingParts(parts, thinkingEnabled, newType, bodyStarted, state = {}) {
  state.thinkingLiteral ||= new LiteralState();
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
      const cleaned = p.text;
      if (cleaned) {
        out.push({ ...p, text: cleaned });
      }
    }
    return { parts: out, newType, detectionThinking: parts.filter(p => p?.type === 'thinking').map(p => p.text).join('') };
  }
  const splitResult = splitThinkingParts(parts, state);
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
  flushThinkingState,
};
