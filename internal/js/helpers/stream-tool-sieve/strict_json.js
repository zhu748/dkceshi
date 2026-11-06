'use strict';

// JSON.parse silently chooses the last duplicate key. Match Go's lossless
// decoder and reject ambiguous objects before any local tool interpretation.
function parseJSONStrict(raw) {
  const value = JSON.parse(raw);
  const stack = [];
  for (let i = 0; i < raw.length; i += 1) {
    switch (raw[i]) {
      case '{': stack.push({ keys: new Set(), key: true }); break;
      case '[': stack.push({ keys: null, key: false }); break;
      case '}': case ']': stack.pop(); break;
      case ':': if (stack.length) stack.at(-1).key = false; break;
      case ',': if (stack.at(-1)?.keys) stack.at(-1).key = true; break;
      case '"': {
        const start = i;
        for (i += 1; i < raw.length; i += 1) {
          if (raw[i] === '\\') { i += 1; continue; }
          if (raw[i] === '"') break;
        }
        const key = JSON.parse(raw.slice(start, i + 1));
        if (!wellFormedString(key)) throw new SyntaxError('invalid Unicode in JSON string');
        const scope = stack.at(-1);
        if (!scope?.key) break;
        if (scope.keys.has(key)) throw new SyntaxError('duplicate JSON object key');
        scope.keys.add(key);
        break;
      }
    }
  }
  return value;
}

function hasDuplicateJSONKeys(raw) {
  try { parseJSONStrict(raw); } catch (error) {
    return error.message === 'duplicate JSON object key';
  }
  return false;
}

function wellFormedString(text) {
  for (let i = 0; i < text.length; i++) {
    const code = text.charCodeAt(i);
    if (code >= 0xdc00 && code <= 0xdfff) return false;
    if (code < 0xd800 || code > 0xdbff) continue;
    const low = text.charCodeAt(++i);
    if (!(low >= 0xdc00 && low <= 0xdfff)) return false;
  }
  return true;
}

function hasInvalidJSONLiteral(raw) {
  try { JSON.parse(raw); } catch { return false; }
  try { parseJSONStrict(raw); } catch { return true; }
  return false;
}

module.exports = { parseJSONStrict, hasDuplicateJSONKeys, hasInvalidJSONLiteral };
