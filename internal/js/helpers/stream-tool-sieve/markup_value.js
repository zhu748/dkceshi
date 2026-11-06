'use strict';
const { parseJSONStrict } = require('./strict_json');
const { parameterMarkupLimitExceeded } = require('./parameter_markup_budget');
const { emptyToolMarkupGap } = require('./body_integrity');

const { toRawString } = require('./state');
const { unescapeHtml } = require('./entities');
const { parseMixedCDATAString, extractStandaloneCDATA } = require('./mixed_cdata');
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
  const kv = parseMarkupKVObject(s);
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
  const counts = new Map();
  let prev = 0;
  for (const block of findGenericXmlElementBlocks(raw)) {
    if (!emptyToolMarkupGap(raw.slice(prev, block.start))) return {};
    const key = toRawString(block.localName).trim();
    if (!key) {
      continue;
    }
    const value = parseMarkupValue(block.body, key, true);
    if (value === undefined) {
      continue;
    }
    appendMarkupValue(out, counts, key, value);
    prev = block.end;
  }
  if (!emptyToolMarkupGap(raw.slice(prev))) return {};
  if (Object.keys(out).length === 1 && Object.hasOwn(out, 'item')) {
    return counts.get('item') === 1 ? [out.item] : out.item;
  }
  return out;
}

function parseMarkupValue(raw, paramName = '', literalCDATA = false) {
  const cdata = extractStandaloneCDATA(raw);
  if (cdata.ok) {
    if (parameterMarkupLimitExceeded(cdata.value)) return cdata.value;
    if (literalCDATA || preservesCDATAStringParameter(paramName)) return cdata.value;
    const literal = parseJSONLiteralValue(cdata.value);
    if (literal.ok) {
      return literal.value;
    }
    const structured = parseStructuredCDATAParameterValue(paramName, cdata.value);
    if (structured.ok) {
      return structured.value;
    }
    const looseArray = parseLooseJSONArrayValue(cdata.value, paramName);
    return looseArray.ok ? looseArray.value : cdata.value;
  }
  const mixed = parseMixedCDATAString(toRawString(raw).trim());
  if (mixed.ok) {
    if (literalCDATA || preservesCDATAStringParameter(paramName)) return mixed.value;
    const literal = parseJSONLiteralValue(mixed.value);
    return literal.ok ? literal.value : mixed.value;
  }
  const exact = parseJSONLiteralValue(toRawString(raw).trim());
  if (exact.ok) {
    return exact.value;
  }
  const source = toRawString(raw).trim();
  const s = source.includes('<') && source.includes('>') ? source : toRawString(extractRawTagValue(raw)).trim();
  if (!s) {
    return '';
  }

  if (s.includes('<') && s.includes('>')) {
    const nested = parseMarkupInput(s);
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
      const parsed = parseJSONStrict(repairedBackslashes);
      return { ok: true, value: parsed };
    } catch (_err) {
      // Fall through.
    }
  }

  const repairedLoose = repairLooseJSON(s);
  if (repairedLoose !== s) {
    try {
      const parsed = parseJSONStrict(repairedLoose);
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
      const parsed = parseJSONStrict(raw);
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
    const parsed = parseJSONStrict(JSON.stringify(v));
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed;
    }
  } catch (_err) {
    return {};
  }
  return {};
}

function appendMarkupValue(out, counts, key, value) {
  const count = counts.get(key) || 0;
  if (count >= 2) out[key].push(value);
  else Object.defineProperty(out, key, { value: count === 0 ? value : [out[key], value], enumerable: true, configurable: true, writable: true });
  counts.set(key, count + 1);
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
