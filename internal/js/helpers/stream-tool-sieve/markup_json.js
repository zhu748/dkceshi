'use strict';

// JSON literal parsing, array coercion and conservative JSON repairs.
const { toRawString } = require('./state');

function parseJSONLiteralValue(raw) {
  const s = toRawString(raw).trim();
  if (!s) {
    return { ok: false, value: null };
  }
  if (!['{', '[', '"', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 't', 'f', 'n'].includes(s[0])) {
    return { ok: false, value: null };
  }
  try {
    return { ok: true, value: JSON.parse(s) };
  } catch (_err) {
    return { ok: false, value: null };
  }
}

function coerceArrayValue(value, paramName = '') {
  if (Array.isArray(value)) {
    return { ok: true, value };
  }
  if (!value || typeof value !== 'object') {
    return { ok: false, value: null };
  }

  const keys = Object.keys(value);
  if (keys.length !== 1) {
    return { ok: false, value: null };
  }

  if (Object.prototype.hasOwnProperty.call(value, 'item')) {
    const items = value.item;
    const nested = coerceArrayValue(items, '');
    return nested.ok ? nested : { ok: true, value: [items] };
  }

  if (paramName && Object.prototype.hasOwnProperty.call(value, paramName)) {
    const nested = coerceArrayValue(value[paramName], '');
    if (nested.ok) {
      return nested;
    }
  }

  return { ok: false, value: null };
}

function splitTopLevelJSONValues(raw) {
  const s = toRawString(raw).trim();
  if (!s) {
    return [];
  }

  const values = [];
  let start = 0;
  let depth = 0;
  let inString = false;
  let escaped = false;

  for (let i = 0; i < s.length; i += 1) {
    const ch = s[i];
    if (inString) {
      if (escaped) {
        escaped = false;
        continue;
      }
      if (ch === '\\') {
        escaped = true;
        continue;
      }
      if (ch === '"') {
        inString = false;
      }
      continue;
    }
    if (ch === '"') {
      inString = true;
      continue;
    }
    if (ch === '{' || ch === '[') {
      depth += 1;
      continue;
    }
    if (ch === '}' || ch === ']') {
      if (depth > 0) {
        depth -= 1;
      }
      continue;
    }
    if (ch === ',' && depth === 0) {
      const segment = s.slice(start, i).trim();
      if (!segment) {
        return [];
      }
      values.push(segment);
      start = i + 1;
    }
  }

  const last = s.slice(start).trim();
  if (!last) {
    return [];
  }
  values.push(last);
  return values.length > 1 ? values : [];
}

function repairInvalidJSONBackslashes(s) {
  if (!s || !s.includes('\\')) {
    return s;
  }

  let out = '';
  for (let i = 0; i < s.length; i += 1) {
    const ch = s[i];
    if (ch !== '\\') {
      out += ch;
      continue;
    }
    if (i + 1 < s.length) {
      const next = s[i + 1];
      if ('"\\/bfnrt'.includes(next)) {
        out += `\\${next}`;
        i += 1;
        continue;
      }
      if (next === 'u' && i + 5 < s.length) {
        let isHex = true;
        for (let j = 1; j <= 4; j += 1) {
          const r = s[i + 1 + j];
          if (!/[0-9a-fA-F]/.test(r)) {
            isHex = false;
            break;
          }
        }
        if (isHex) {
          out += `\\u${s.slice(i + 2, i + 6)}`;
          i += 5;
          continue;
        }
      }
    }
    out += '\\\\';
  }
  return out;
}

function repairLooseJSON(s) {
  const raw = toRawString(s).trim();
  if (!raw) {
    return raw;
  }
  let out = raw.replace(/([{,]\s*)([a-zA-Z_][a-zA-Z0-9_]*)\s*:/g, '$1"$2":');
  out = out.replace(/(:\s*)(\{(?:[^{}]|\{[^{}]*\})*\}(?:\s*,\s*\{(?:[^{}]|\{[^{}]*\})*\})+)/g, '$1[$2]');
  return out;
}

module.exports = {
  coerceArrayValue,
  parseJSONLiteralValue,
  repairInvalidJSONBackslashes,
  repairLooseJSON,
  splitTopLevelJSONValues,
};
