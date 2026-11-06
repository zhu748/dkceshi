'use strict';

const { skipXmlIgnoredSection, toolCDATAOpenLenAt } = require('./cdata_scan');
const { findXmlTagEnd } = require('./markup_syntax');

function parameterMarkupLimitExceeded(text) {
  let depth = 0, nodes = 0;
  for (let i = 0; i < text.length;) {
    const skipped = skipXmlIgnoredSection(text, i);
    if (skipped.blocked) return false;
    if (skipped.advanced) { i = skipped.next; continue; }
    if (text[i] !== '<') { i++; continue; }
    let start = i + 1;
    const closing = text[start] === '/';
    if (closing) start++;
    if (!(text.charCodeAt(start) >= 0x80 || /[A-Za-z_:]/.test(text[start] || ''))) { i++; continue; }
    const end = findXmlTagEnd(text, start);
    if (end < 0) return false;
    nodes++;
    if (closing) depth = Math.max(0, depth - 1);
    else if (!text.slice(i, end).trimEnd().endsWith('/')) depth++;
    if (depth > 128 || nodes > 32768) return true;
    i = end + 1;
  }
  return false;
}

function parameterXMLNeedsLiteral(text) {
  const stack = [{ child: false, text: false }];
  const markText = value => {
    const current = stack[stack.length - 1];
    current.text ||= value.trim() !== '';
    return current.child && current.text;
  };
  for (let i = 0; i < text.length;) {
    const skipped = skipXmlIgnoredSection(text, i);
    if (skipped.blocked) return false;
    if (skipped.advanced) {
      if (!toolCDATAOpenLenAt(text, i)) return true;
      if (markText(text.slice(i, skipped.next))) return true;
      i = skipped.next; continue;
    }
    if (text[i] !== '<') {
      const found = text.indexOf('<', i);
      const end = found < 0 ? text.length : found;
      if (markText(text.slice(i, end))) return true;
      i = end; continue;
    }
    let start = i + 1;
    const closing = text[start] === '/';
    if (closing) start++;
    if (!(text.charCodeAt(start) >= 0x80 || /[A-Za-z_:]/.test(text[start] || ''))) {
      if (markText('<')) return true;
      i++; continue;
    }
    const end = findXmlTagEnd(text, start);
    if (end < 0) return false;
    const body = text.slice(start, end).trim().replace(/\/$/, '').trim();
    if (/[\s:]/.test(body)) return true;
    if (closing) {
      if (stack.length > 1) stack.pop();
    } else {
      const current = stack[stack.length - 1];
      current.child = true;
      if (current.text) return true;
      if (!text.slice(start, end).trimEnd().endsWith('/')) stack.push({ child: false, text: false });
    }
    i = end + 1;
  }
  return false;
}

module.exports = { parameterMarkupLimitExceeded, parameterXMLNeedsLiteral };
