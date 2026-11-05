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
        const scope = stack.at(-1);
        if (!scope?.key) break;
        const key = JSON.parse(raw.slice(start, i + 1));
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

module.exports = { parseJSONStrict, hasDuplicateJSONKeys };
