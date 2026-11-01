'use strict';

const { findToolMarkupTagOutsideIgnored, findMatchingToolMarkupClose } = require('./stream-tool-sieve/markup_scan');

const unit = (end = 0, text = '', wait = false) => ({ end, text, wait });
const { compactTag, displayTagEnd, displayWrapperClose } = require('./visible-cleaner-boundary');
const isAgentOpen = tag => ['<attempt_completion>', '<ask_followup_question>', '<new_task>'].includes(tag);
const isAgentClose = tag => tag.startsWith('</') && isAgentOpen('<' + tag.slice(2));

function cleanDisplayUnit(raw, final, cleanVisible) {
  for (const [open, close] of [['<!--', '-->'], ['<![CDATA[', ']]>']]) {
    if (open.startsWith(raw) && !final) return unit(0, '', true);
    if (raw.startsWith(open)) {
      let end = raw.indexOf(close, open.length);
      if (end < 0) return unit(raw.length, raw, !final);
      end += close.length;
      return unit(end, raw.slice(0, end));
    }
  }
  if (raw[0] === '"') {
    for (let i = 1; i < raw.length; i++) {
      if (raw[i] === '\\') { i++; continue; }
      if (raw.charCodeAt(i) < 32) return unit(1, raw.slice(0, 1));
      if (raw[i] === '"') return unit(i + 1, raw.slice(0, i + 1));
    }
    return unit(raw.length, raw, !final);
  }
  if (raw[0] === '[') {
    const tail = raw.slice(1).trimStart();
    if (!tail) return unit(raw.length, raw, !final);
    if (tail[0] === '{') {
      const value = jsonValue(raw);
      if (value.incomplete && !final) return unit(0, '', true);
      if (value.end) {
        const calls = value.value;
        const wire = Array.isArray(calls) && calls.length > 0 && calls.every(call =>
          call && typeof call.id === 'string' && call.id.startsWith('call') && call.type === 'function' &&
          call.function && typeof call.function.name === 'string' && call.function.name && Object.hasOwn(call.function, 'arguments'));
        return unit(value.end, wire ? '' : raw.slice(0, value.end));
      }
    }
  }
  if (raw[0] !== '<') return unit();
  const end = displayTagEnd(raw);
  if (!end) return unit(raw.length, raw, !final);
  const tag = compactTag(raw.slice(0, end));
  if (tag === '<think>' || isAgentOpen(tag)) {
    const { start: close, end: closeEnd } = displayWrapperClose(raw, end, tag);
    if (close < 0 && !final) return unit(0, '', true);
    if (close < 0 && tag === '<think>') return unit(raw.length);
    let body = close < 0 ? raw.slice(end) : raw.slice(end, close);
    if (isAgentOpen(tag)) { body = body.replace(/^<result>/, ''); if (close >= 0) body = body.replace(/<\/result>$/, ''); }
    return unit(close < 0 ? raw.length : closeEnd, cleanVisible(body));
  }
  if (tag === '</result>') {
    const tail = raw.slice(end).trimStart();
    if (!tail && !final) return unit(0, '', true);
    if (tail.startsWith('<')) {
      const n = displayTagEnd(tail);
      if (!n && !final) return unit(0, '', true);
      if (n && isAgentClose(compactTag(tail.slice(0, n)))) return unit(raw.length - tail.length + n);
    }
  }
  if (isAgentClose(tag)) return unit(end);
  if (tag === '<|tool|>') {
    const tail = raw.slice(end).trimStart();
    if (!tail && !final) return unit(0, '', true);
    if (tail.startsWith('{')) {
      const value = jsonValue(tail);
      if (value.incomplete && !final) return unit(0, '', true);
      if (value.end && typeof value.value?.tool_call_id === 'string' && value.value.tool_call_id.startsWith('call')) return unit(raw.length - tail.length + value.end);
    }
  }
  const open = findToolMarkupTagOutsideIgnored(raw, 0);
  if (open?.start === 0) {
    if (open.closing && open.epseLike) return unit(open.end + 1);
    if (!open.closing && open.name === 'tool_calls') {
      const close = findMatchingToolMarkupClose(raw, open);
      if (close) return unit(close.end + 1);
      if (!final) return unit(0, '', true);
    }
  }
  return unit(end, cleanControlToken(raw.slice(0, end)));
}

function cleanControlToken(text) {
  if (/^<\/?\s*think\s*>$/i.test(text)) return '';
  if (/^<[|｜]\s*(?:begin[_▁]of[_▁]sentence|(?:begin[_▁])?[_▁]*of[_▁]thought|assistant|tool|end[_▁]of[_▁](?:sentence|thinking|thought|toolresults|instructions))\s*[|｜]>$/i.test(text)) return '';
  return text;
}

function jsonValue(text) {
  const stack = []; let quote = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (quote) { if (ch === '\\') i++; else if (ch === '"') quote = false; continue; }
    if (ch === '"') quote = true;
    else if (ch === '[' || ch === '{') stack.push(ch);
    else if (ch === ']' || ch === '}') {
      if (stack.pop() !== (ch === ']' ? '[' : '{')) return { end: 0 };
      if (!stack.length) {
        try { return { end: i + 1, value: JSON.parse(text.slice(0, i + 1)) }; }
        catch { return { end: 0 }; }
      }
    }
  }
  return { end: 0, incomplete: true };
}

module.exports = { cleanDisplayUnit };
