'use strict';

// Recursive parameter-value grammar, preserving literal code and CDATA.
const { toRawString } = require('./state');
const {
  findToolCDATAEnd,
  findTrailingToolCDATACloseStart,
  toolCDATAOpenLenAt,
} = require('./cdata_scan');
const {
  findGenericXmlElementBlocks,
} = require('./markup_elements');
const {
  coerceArrayValue,
  parseJSONLiteralValue,
  repairInvalidJSONBackslashes,
  repairLooseJSON,
  splitTopLevelJSONValues,
} = require('./markup_json');

function parseMarkupInput(raw) {
  const s = toRawString(raw).trim();
  if (!s) {
    return {};
  }
  // Prioritize XML-style KV tags (e.g., <arg>val</arg>)
  const kv = unwrapItemOnlyMarkupValue(parseMarkupKVObject(s));
  if (Array.isArray(kv)) {
    return kv;
  }
  if (kv && typeof kv === 'object' && Object.keys(kv).length > 0) {
    return kv;
  }

  // Fallback to JSON parsing
  const parsed = parseToolCallInput(s);
  if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
    if (Object.keys(parsed).length > 0) {
      return parsed;
    }
  }

  return { _raw: extractRawTagValue(s) };
}

function parseMarkupKVObject(text) {
  const raw = toRawString(text).trim();
  if (!raw) {
    return {};
  }
  const out = {};
  for (const block of findGenericXmlElementBlocks(raw)) {
    const key = toRawString(block.localName).trim();
    if (!key) {
      continue;
    }
    const value = parseMarkupValue(block.body, key);
    if (value === undefined || value === null) {
      continue;
    }
    appendMarkupValue(out, key, value);
  }
  return out;
}

function parseMarkupValue(raw, paramName = '') {
  const cdata = extractStandaloneCDATA(raw);
  if (cdata.ok) {
    const literal = parseJSONLiteralValue(cdata.value);
    if (literal.ok) {
      const literalArray = coerceArrayValue(literal.value, paramName);
      if (literalArray.ok) {
        return literalArray.value;
      }
      return literal.value;
    }
    const structured = parseStructuredCDATAParameterValue(paramName, cdata.value);
    if (structured.ok) {
      return structured.value;
    }
    const looseArray = parseLooseJSONArrayValue(cdata.value, paramName);
    return looseArray.ok ? looseArray.value : cdata.value;
  }
  const s = toRawString(extractRawTagValue(raw)).trim();
  if (!s) {
    return '';
  }

  if (s.includes('<') && s.includes('>')) {
    const nested = unwrapItemOnlyMarkupValue(parseMarkupInput(s));
    if (Array.isArray(nested)) {
      return nested;
    }
    if (nested && typeof nested === 'object') {
      const nestedArray = coerceArrayValue(nested, paramName);
      if (nestedArray.ok) {
        return nestedArray.value;
      }
      if (isOnlyRawValue(nested)) {
        const rawValue = toRawString(nested._raw);
        const looseArray = parseLooseJSONArrayValue(rawValue, paramName);
        return looseArray.ok ? looseArray.value : rawValue;
      }
      return nested;
    }
  }

  const literal = parseJSONLiteralValue(s);
  if (literal.ok) {
    const literalArray = coerceArrayValue(literal.value, paramName);
    if (literalArray.ok) {
      return literalArray.value;
    }
    return literal.value;
  }
  const looseArray = parseLooseJSONArrayValue(s, paramName);
  if (looseArray.ok) {
    return looseArray.value;
  }
  return s;
}

function parseStructuredCDATAParameterValue(paramName, raw) {
  if (preservesCDATAStringParameter(paramName)) {
    return { ok: false, value: null };
  }
  const normalized = normalizeCDATAForStructuredParse(raw);
  if (!normalized.includes('<') || !normalized.includes('>')) {
    return { ok: false, value: null };
  }
  if (!cdataFragmentLooksExplicitlyStructured(normalized)) {
    return { ok: false, value: null };
  }
  const parsed = parseMarkupInput(normalized);
  if (Array.isArray(parsed)) {
    return { ok: true, value: parsed };
  }
  if (parsed && typeof parsed === 'object' && !isOnlyRawValue(parsed) && Object.keys(parsed).length > 0) {
    return { ok: true, value: parsed };
  }
  return { ok: false, value: null };
}

function normalizeCDATAForStructuredParse(raw) {
  return unescapeHtml(toRawString(raw).replace(/<br\s*\/?>/gi, '\n').trim());
}

function cdataFragmentLooksExplicitlyStructured(raw) {
  const blocks = findGenericXmlElementBlocks(raw);
  if (blocks.length === 0) {
    return false;
  }
  if (blocks.length > 1) {
    return true;
  }
  const block = blocks[0];
  if (toRawString(block.localName).trim().toLowerCase() === 'item') {
    return true;
  }
  return findGenericXmlElementBlocks(block.body).length > 0;
}

function preservesCDATAStringParameter(name) {
  return new Set([
    'content',
    'file_content',
    'text',
    'prompt',
    'query',
    'command',
    'cmd',
    'script',
    'code',
    'old_string',
    'new_string',
    'pattern',
    'path',
    'file_path',
  ]).has(toRawString(name).trim().toLowerCase());
}

function unwrapItemOnlyMarkupValue(value) {
  if (Array.isArray(value)) {
    return value.map(unwrapItemOnlyMarkupValue);
  }
  if (!value || typeof value !== 'object') {
    return value;
  }
  const keys = Object.keys(value);
  if (keys.length === 1 && keys[0] === 'item') {
    const items = unwrapItemOnlyMarkupValue(value.item);
    return Array.isArray(items) ? items : [items];
  }
  const out = {};
  for (const key of keys) {
    out[key] = unwrapItemOnlyMarkupValue(value[key]);
  }
  return out;
}

function extractRawTagValue(inner) {
  const s = toRawString(inner).trim();
  if (!s) {
    return '';
  }

  // 1. Check for CDATA
  const cdata = extractStandaloneCDATA(s);
  if (cdata.ok) {
    return cdata.value;
  }

  // 2. Fallback to unescaping standard HTML entities
  // Note: we avoid broad tag stripping here to preserve user content (like < symbols in code)
  return unescapeHtml(inner);
}

function unescapeHtml(safe) {
  if (!safe) return '';
  return safe.replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#039;/g, "'")
    .replace(/&#x27;/g, "'");
}

function extractStandaloneCDATA(inner) {
  const s = toRawString(inner).trim();
  const openLen = toolCDATAOpenLenAt(s, 0);
  if (!openLen) {
    return { ok: false, value: '' };
  }
  const closeStart = findTrailingToolCDATACloseStart(s);
  if (closeStart >= openLen) {
    return { ok: true, value: s.slice(openLen, closeStart) };
  }
  const end = findToolCDATAEnd(s, openLen);
  if (end >= 0) {
    return { ok: true, value: s.slice(openLen, end) };
  }
  return { ok: true, value: s.slice(openLen) };
}

function parseLooseJSONArrayValue(raw, paramName = '') {
  if (preservesCDATAStringParameter(paramName)) {
    return { ok: false, value: null };
  }
  const s = toRawString(raw).trim();
  if (!s) {
    return { ok: false, value: null };
  }
  const candidate = parseLooseJSONArrayCandidate(s, paramName);
  if (candidate.ok) {
    return candidate;
  }

  const segments = splitTopLevelJSONValues(s);
  if (segments.length < 2) {
    return { ok: false, value: null };
  }

  const out = [];
  for (const segment of segments) {
    const parsed = parseLooseArrayElementValue(segment);
    if (!parsed.ok) {
      return { ok: false, value: null };
    }
    out.push(parsed.value);
  }
  return { ok: true, value: out };
}

function parseLooseJSONArrayCandidate(raw, paramName = '') {
  const parsed = parseLooseArrayElementValue(raw);
  if (!parsed.ok) {
    return { ok: false, value: null };
  }
  return coerceArrayValue(parsed.value, paramName);
}

function parseLooseArrayElementValue(raw) {
  const s = toRawString(raw).trim();
  if (!s) {
    return { ok: false, value: null };
  }

  const literal = parseJSONLiteralValue(s);
  if (literal.ok) {
    return literal;
  }

  const repairedBackslashes = repairInvalidJSONBackslashes(s);
  if (repairedBackslashes !== s) {
    try {
      const parsed = JSON.parse(repairedBackslashes);
      return { ok: true, value: parsed };
    } catch (_err) {
      // Fall through.
    }
  }

  const repairedLoose = repairLooseJSON(s);
  if (repairedLoose !== s) {
    try {
      const parsed = JSON.parse(repairedLoose);
      return { ok: true, value: parsed };
    } catch (_err) {
      // Fall through.
    }
  }

  if (s.includes('<') && s.includes('>')) {
    const parsed = parseMarkupInput(s);
    if (Array.isArray(parsed)) {
      return { ok: true, value: parsed };
    }
    if (parsed && typeof parsed === 'object') {
      return { ok: true, value: parsed };
    }
  }

  return { ok: false, value: null };
}

function parseToolCallInput(v) {
  if (v == null) {
    return {};
  }
  if (typeof v === 'string') {
    const raw = toRawString(v);
    if (!raw) {
      return {};
    }
    try {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed;
      }
      return { _raw: raw };
    } catch (_err) {
      return { _raw: raw };
    }
  }
  if (typeof v === 'object' && !Array.isArray(v)) {
    return v;
  }
  try {
    const parsed = JSON.parse(JSON.stringify(v));
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed;
    }
  } catch (_err) {
    return {};
  }
  return {};
}

function appendMarkupValue(out, key, value) {
  if (Object.prototype.hasOwnProperty.call(out, key)) {
    const current = out[key];
    if (Array.isArray(current)) {
      current.push(value);
      return;
    }
    out[key] = [current, value];
    return;
  }
  out[key] = value;
}

function isOnlyRawValue(obj) {
  if (!obj || typeof obj !== 'object' || Array.isArray(obj)) {
    return false;
  }
  const keys = Object.keys(obj);
  return keys.length === 1 && keys[0] === '_raw';
}

module.exports = {
  appendMarkupValue,
  parseMarkupValue,
};
