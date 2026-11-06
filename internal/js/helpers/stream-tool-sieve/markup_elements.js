'use strict';

const { toRawString } = require('./state');
const { skipXmlIgnoredSection } = require('./cdata_scan');
const { findXmlTagEnd, hasXmlTagBoundary, isSelfClosingXmlTag } = require('./markup_syntax');

// Every opener is matched once. Unclosed ancestors cannot cause repeated
// suffix scans; a complete descendant still remains available for diagnostics.
function collectElements(source, wanted = '') {
  const blocks = [];
  const stacks = new Map();
  let depth = 0;
  for (let i = 0; i < source.length;) {
    const skipped = skipXmlIgnoredSection(source, i);
    if (skipped.blocked) break;
    if (skipped.advanced) { i = skipped.next; continue; }
    if (source[i] !== '<') { i++; continue; }
    const start = i;
    const closing = source[i + 1] === '/';
    const nameStart = i + (closing ? 2 : 1);
    const match = source.slice(nameStart).match(/^([A-Za-z_][A-Za-z0-9_.:-]*)/);
    if (!match) { i++; continue; }
    const name = match[1];
    const nameEnd = nameStart + name.length;
    if (!hasXmlTagBoundary(source, nameEnd)) { i++; continue; }
    const end = findXmlTagEnd(source, nameEnd);
    if (end < 0) break;
    i = end + 1;
    const key = name.toLowerCase();
    if (wanted && key !== wanted) continue;
    const stack = stacks.get(key) || [];
    if (closing) {
      const index = stack.pop();
      if (index !== undefined) {
        depth--;
        blocks[index].body = source.slice(blocks[index].bodyStart, start);
        blocks[index].end = i;
      }
      continue;
    }
    const selfClosing = isSelfClosingXmlTag(source.slice(start, end));
    if (wanted && selfClosing) continue;
    if (blocks.length >= 32768 || (!selfClosing && depth >= 128)) return [];
    const block = { name, localName: name.includes(':') ? name.slice(name.lastIndexOf(':') + 1) : name,
      attrs: source.slice(nameEnd, end), body: '', bodyStart: i, start, end: selfClosing ? i : 0 };
    if (!selfClosing) { stack.push(blocks.length); depth++; }
    stacks.set(key, stack);
    blocks.push(block);
  }
  const out = [];
  let end = 0;
  for (const block of blocks) {
    if (block.end && block.start >= end) {
      const { bodyStart, ...result } = block;
      out.push(result);
      end = block.end;
    }
  }
  return out;
}

function findXmlElementBlocks(text, tag) {
  return collectElements(toRawString(text), toRawString(tag).toLowerCase());
}

function findGenericXmlElementBlocks(text) {
  return collectElements(toRawString(text));
}

module.exports = { findGenericXmlElementBlocks, findXmlElementBlocks };
