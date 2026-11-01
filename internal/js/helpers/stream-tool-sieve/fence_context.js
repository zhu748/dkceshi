'use strict';

const { findToolMarkupTagOutsideIgnored } = require('./markup_scan');

function openToolParameterBeforeFence(text) {
  let depth = 0;
  for (let offset = 0; offset < text.length;) {
    const tag = findToolMarkupTagOutsideIgnored(text, offset);
    if (!tag) break;
    if (tag.name === 'parameter') depth = tag.closing ? Math.max(0, depth - 1) : depth + 1;
    offset = tag.end + 1;
  }
  return depth > 0;
}

module.exports = { openToolParameterBeforeFence };
