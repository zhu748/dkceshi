'use strict';

// Upstream content-filter status and leaked suffix handling.
const { asString } = require('./sse_text');

function filterLeakedContentFilterParts(parts) {
  if (!Array.isArray(parts) || parts.length === 0) {
    return parts;
  }
  const out = [];
  for (const p of parts) {
    if (!p || typeof p !== 'object') {
      continue;
    }
    const { text, stripped } = stripLeakedContentFilterSuffix(p.text);
    if (stripped && shouldDropCleanedLeakedChunk(text)) {
      continue;
    }
    if (stripped) {
      out.push({ ...p, text });
      continue;
    }
    out.push(p);
  }
  return out;
}

function stripLeakedContentFilterSuffix(text) {
  if (typeof text !== 'string' || text === '') {
    return { text, stripped: false };
  }
  const upperText = text.toUpperCase();
  const idx = upperText.indexOf('CONTENT_FILTER');
  if (idx < 0) {
    return { text, stripped: false };
  }
  return {
    text: text.slice(0, idx).replace(/[ \t\r]+$/g, ''),
    stripped: true,
  };
}

function shouldDropCleanedLeakedChunk(cleaned) {
  if (cleaned === '') {
    return true;
  }
  if (typeof cleaned === 'string' && cleaned.includes('\n')) {
    return false;
  }
  return asString(cleaned).trim() === '';
}

function hasContentFilterStatus(chunk) {
  if (!chunk || typeof chunk !== 'object') {
    return false;
  }
  const code = asString(chunk.code);
  if (code && code.toLowerCase() === 'content_filter') {
    return true;
  }
  return hasContentFilterStatusValue(chunk);
}

function hasContentFilterStatusValue(v) {
  if (Array.isArray(v)) {
    for (const item of v) {
      if (hasContentFilterStatusValue(item)) {
        return true;
      }
    }
    return false;
  }
  if (!v || typeof v !== 'object') {
    return false;
  }
  const pathValue = asString(v.p);
  if (pathValue && pathValue.toLowerCase().includes('status')) {
    if (asString(v.v).toLowerCase() === 'content_filter') {
      return true;
    }
  }
  if (asString(v.code).toLowerCase() === 'content_filter') {
    return true;
  }
  for (const value of Object.values(v)) {
    if (hasContentFilterStatusValue(value)) {
      return true;
    }
  }
  return false;
}

module.exports = {
  filterLeakedContentFilterParts,
  hasContentFilterStatus,
};
