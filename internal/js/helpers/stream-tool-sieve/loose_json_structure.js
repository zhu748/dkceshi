'use strict';

// Same-offset masks allow structural insertions without editing quoted data.
function structureMask(raw) {
  const mask = raw.split('');
  let quoted = false, escaped = false;
  for (let i = 0; i < raw.length; i++) {
    const ch = raw[i];
    if (!quoted) { quoted = ch === '"'; continue; }
    mask[i] = ' ';
    if (escaped) escaped = false;
    else if (ch === '\\') escaped = true;
    else if (ch === '"') { mask[i] = '"'; quoted = false; }
  }
  return quoted ? null : mask.join('');
}

function insertStructure(raw, edits) {
  if (!edits.size) return raw;
  const parts = [];
  let start = 0;
  for (const offset of [...edits.keys()].sort((a, b) => a - b)) {
    parts.push(raw.slice(start, offset), edits.get(offset));
    start = offset;
  }
  parts.push(raw.slice(start));
  return parts.join('');
}

function quoteKeys(raw) {
  const mask = structureMask(raw);
  if (mask === null) return raw;
  const edits = new Map();
  for (const match of mask.matchAll(/([{,]\s*)([a-zA-Z_][a-zA-Z0-9_]*)\s*:/g)) {
    const start = match.index + match[1].length;
    edits.set(start, '"');
    edits.set(start + match[2].length, '"');
  }
  return insertStructure(raw, edits);
}

function bracketObjectLists(raw) {
  const mask = structureMask(raw);
  if (mask === null) return raw;
  const ends = new Map(), stack = [];
  for (let i = 0; i < mask.length; i++) {
    const ch = mask[i];
    if (ch === '{' || ch === '[') stack.push(i);
    else if (ch === '}' || ch === ']') {
      if (!stack.length) return raw;
      const start = stack.pop();
      if ((mask[start] === '{' && ch !== '}') || (mask[start] === '[' && ch !== ']')) return raw;
      ends.set(start, i);
    }
  }
  const space = pos => {
    while (pos < mask.length && ' \t\r\n'.includes(mask[pos])) pos++;
    return pos;
  };
  const edits = new Map();
  for (let i = 0; i < mask.length; i++) {
    if (mask[i] !== ':') continue;
    const start = space(i + 1);
    let end = ends.get(start), count = 1;
    if (end === undefined || mask[start] !== '{') continue;
    for (let next = space(end + 1); mask[next] === ','; next = space(end + 1)) {
      const object = space(next + 1), last = ends.get(object);
      if (last === undefined || mask[object] !== '{') break;
      end = last; count++;
    }
    if (count > 1) { edits.set(start, '['); edits.set(end + 1, ']'); }
  }
  return insertStructure(raw, edits);
}

function repairLooseJSON(raw) { return bracketObjectLists(quoteKeys(raw.trim())); }

module.exports = { repairLooseJSON };
