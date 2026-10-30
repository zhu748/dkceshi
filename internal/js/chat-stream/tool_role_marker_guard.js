'use strict';

const TOOL_ROLE_MARKER = '[Tool]:';
function createToolRoleMarkerGuard() {
  return { pending: '', tripped: false, cdata: false, quote: false, escaped: false, code: '', ticks: 0, fence: false, fenceClosing: false, notLineStart: false };
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
    if (tail.slice(0, TOOL_ROLE_MARKER.length).toLowerCase() === TOOL_ROLE_MARKER.toLowerCase()) { guard.tripped = true; return data.slice(0, i); }
    if (!final && tail.length < TOOL_ROLE_MARKER.length && TOOL_ROLE_MARKER.toLowerCase().startsWith(tail.toLowerCase())) break;
    if (data[i] === '\n' || data[i] === '\r') guard.notLineStart = false;
    else if (data[i] !== ' ' && data[i] !== '\t') guard.notLineStart = true;
    i++;
  }
  guard.pending = data.slice(i);
  return data.slice(0, i);
}
function feedToolRoleMarkerGuard(guard, text) { return scan(guard, text); }
function indexToolRoleMarker(text) {
  const guard = createToolRoleMarkerGuard();
  const kept = scan(guard, text, true);
  return guard.tripped ? kept.length : -1;
}
module.exports = { TOOL_ROLE_MARKER, createToolRoleMarkerGuard, feedToolRoleMarkerGuard, indexToolRoleMarker };
