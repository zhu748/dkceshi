'use strict';

const TOOL_ROLE_MARKER = '[Tool]:';
function createToolRoleMarkerGuard() {
  return { pending: '', tripped: false, reason: '', cdata: false, quote: false, escaped: false, code: '', ticks: 0, fence: false, fenceClosing: false, notLineStart: false };
}

// Keep the serialized terminator grammar in sync with sse/transcript_boundary.go.
function scanTranscriptBoundary(tail) {
  const prefix = '[EndMessage:';
  if (!tail.startsWith('[')) return {};
  if (tail.length < prefix.length) return { partial: prefix.toLowerCase().startsWith(tail.toLowerCase()) };
  if (tail.slice(0, prefix.length).toLowerCase() !== prefix.toLowerCase()) return {};
  for (let i = prefix.length; i < prefix.length + 16; i++) {
    if (i === tail.length) return { partial: true };
    if (!/[0-9a-f]/i.test(tail[i])) return {};
  }
  const end = prefix.length + 16;
  return tail.length === end ? { partial: true } : { matched: tail[end] === ']' };
}
function scan(guard, text, final = false) {
  if (!guard || guard.tripped) return '';
  const data = guard.pending + (typeof text === 'string' ? text : '');
  guard.pending = '';
  let i = 0;
  for (; i < data.length;) {
    const tail = data.slice(i);
    if (guard.cdata) {
      if (tail.startsWith(']]>')) { guard.cdata = false; i += 3; continue; }
      if (!final && ']]>'.startsWith(tail)) break;
      i++; continue;
    }
    if (guard.code) {
      if (guard.fenceClosing) {
        if (data[i] === '\n' || data[i] === '\r') { guard.code = ''; guard.fenceClosing = false; guard.notLineStart = false; i++; continue; }
        if (data[i] === ' ' || data[i] === '\t') { i++; continue; }
        guard.fenceClosing = false;
      }
      if (data[i] === guard.code) {
        let n = 0; while (data[i + n] === guard.code) n++;
        if (!final && i + n === data.length) break;
        if (!guard.fence && n === guard.ticks) { guard.code = ''; guard.ticks = 0; }
        else if (guard.fence && !guard.notLineStart && n >= guard.ticks) guard.fenceClosing = true;
        i += n; guard.notLineStart = true; continue;
      }
      if (data[i] === '\n' || data[i] === '\r') guard.notLineStart = false;
      else if (data[i] !== ' ' && data[i] !== '\t') guard.notLineStart = true;
      i++; continue;
    }
    if (guard.quote) {
      if (guard.escaped) guard.escaped = false;
      else if (data[i] === '\\') guard.escaped = true;
      else if (data[i] === '"') guard.quote = false;
      i++; continue;
    }
    if (tail.startsWith('<![CDATA[')) { guard.cdata = true; i += 9; continue; }
    if (!final && '<![CDATA['.startsWith(tail)) break;
    if (data[i] === '`' || (data[i] === '~' && !guard.notLineStart)) {
      const ch = data[i]; let n = 0; while (data[i + n] === ch) n++;
      if (!final && i + n === data.length) break;
      if (ch === '`' || n >= 3) { guard.code = ch; guard.ticks = n; guard.fence = n >= 3 && !guard.notLineStart; }
      i += n; guard.notLineStart = true; continue;
    }
    if (data[i] === '"') { guard.quote = true; i++; continue; }
    if (tail.slice(0, TOOL_ROLE_MARKER.length).toLowerCase() === TOOL_ROLE_MARKER.toLowerCase()) { guard.tripped = true; guard.reason = 'tool_role_marker'; return data.slice(0, i); }
    const boundary = scanTranscriptBoundary(tail);
    if (boundary.matched) { guard.tripped = true; guard.reason = 'transcript_boundary'; return data.slice(0, i); }
    if (!final && boundary.partial) break;
    if (!final && tail.length < TOOL_ROLE_MARKER.length && TOOL_ROLE_MARKER.toLowerCase().startsWith(tail.toLowerCase())) break;
    if (data[i] === '\n' || data[i] === '\r') guard.notLineStart = false;
    else if (data[i] !== ' ' && data[i] !== '\t') guard.notLineStart = true;
    i++;
  }
  guard.pending = data.slice(i);
  return data.slice(0, i);
}
function feedToolRoleMarkerGuard(guard, text, final = false) { return scan(guard, text, final); }
function indexToolRoleMarker(text) {
  const guard = createToolRoleMarkerGuard();
  const kept = scan(guard, text, true);
  return guard.tripped ? kept.length : -1;
}
module.exports = { TOOL_ROLE_MARKER, createToolRoleMarkerGuard, feedToolRoleMarkerGuard, indexToolRoleMarker };
