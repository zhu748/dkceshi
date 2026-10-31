'use strict';

// Tool tag names, prefixes and confusable-name recognition.
const { toRawString } = require('./state');
const {
  TOOL_MARKUP_NAMES,
  consumeToolKeyword,
  consumeToolMarkupSeparator,
  foldToolKeywordRune,
  normalizeFullwidthASCIIChar,
  normalizedASCIITailAt,
  skipToolMarkupIgnorables,
  toolMarkupDashLenAt,
  toolMarkupUnderscoreLenAt,
  toolMarkupWhitespaceLikeLenAt,
} = require('./markup_syntax');

function consumeToolMarkupNamePrefix(raw, lower, idx) {
  let next = idx;
  let epseLike = false;
  while (true) {
    const consumed = consumeToolMarkupNamePrefixOnce(raw, lower, next);
    if (!consumed.ok) {
      return { next, epseLike };
    }
    next = consumed.next;
    epseLike = true;
  }
}

function matchToolMarkupNameAfterArbitraryPrefix(raw, start) {
  for (let idx = start; idx < raw.length;) {
    if (isToolMarkupTagTerminator(raw, idx)) {
      return { ok: false };
    }
    for (const name of TOOL_MARKUP_NAMES) {
      const matched = consumeToolKeyword(raw, idx, name.raw);
      if (!matched.ok) {
        continue;
      }
      if (!toolMarkupPrefixAllowsLocalNameAt(raw, start, idx)) {
        continue;
      }
      return { ok: true, name: name.canonical, start: idx, len: matched.next - idx };
    }
    idx += 1;
  }
  return { ok: false };
}

function hasPartialToolMarkupNameAfterArbitraryPrefix(raw, start) {
  for (let idx = start; idx < raw.length;) {
    if (isToolMarkupTagTerminator(raw, idx)) {
      return false;
    }
    if (toolMarkupPrefixAllowsLocalNameAt(raw, start, idx) && hasToolMarkupNamePrefix(raw, idx)) {
      return true;
    }
    if (toolMarkupPrefixAllowsLocalNameAt(raw, start, idx) && hasEPSENamePrefixOrPartial(raw, idx)) {
      return true;
    }
    idx += 1;
  }
  return toolMarkupPrefixAllowsLocalName(raw.slice(start));
}

function hasEPSENamePrefixOrPartial(raw, start) {
  const tail = normalizedASCIITailAt(raw, start);
  return tail.startsWith('epse') || 'epse'.startsWith(tail) || hasConfusablePartialKeywordPrefix(raw, start, 'epse');
}

function toolMarkupPrefixAllowsLocalName(prefix) {
  if (!prefix) {
    return false;
  }
  if (normalizedASCIITailAt(prefix, 0).includes('epse')) {
    return true;
  }
  if (/[="']/u.test(prefix)) {
    return false;
  }
  const previous = normalizeFullwidthASCIIChar(prefix[prefix.length - 1] || '');
  return !/^[A-Za-z0-9]$/.test(previous);
}

function toolMarkupPrefixAllowsLocalNameAt(raw, start, localStart) {
  if (start < 0 || localStart <= start || localStart > raw.length) {
    return false;
  }
  const prefix = raw.slice(start, localStart);
  if (toolMarkupPrefixAllowsLocalName(prefix)) {
    return true;
  }
  if (/[="']/u.test(prefix)) {
    return false;
  }
  const previous = normalizeFullwidthASCIIChar(prefix[prefix.length - 1] || '');
  const next = normalizeFullwidthASCIIChar(raw[localStart] || '');
  return /^[A-Za-z0-9]$/.test(previous) && /^[A-Z]$/.test(next);
}

function toolMarkupPrefixContainsSlash(prefix) {
  for (const ch of toRawString(prefix)) {
    if (normalizeFullwidthASCIIChar(ch) === '/') {
      return true;
    }
  }
  return false;
}

function isToolMarkupTagTerminator(raw, idx) {
  return raw[idx] === '>' || normalizeFullwidthASCIIChar(raw[idx] || '') === '>';
}

function consumeToolMarkupNamePrefixOnce(raw, lower, idx) {
  idx = skipToolMarkupIgnorables(raw, idx);
  const sep = consumeToolMarkupSeparator(raw, idx);
  if (sep.ok) {
    return sep;
  }
  const spacingLen = toolMarkupWhitespaceLikeLenAt(raw, idx);
  if (spacingLen > 0) {
    return { next: idx + spacingLen, ok: true };
  }
  const epse = consumeToolKeyword(raw, idx, 'epse');
  if (epse.ok) {
    let next = epse.next;
    const dashLen = toolMarkupDashLenAt(raw, next);
    const underscoreLen = toolMarkupUnderscoreLenAt(raw, next);
    if (dashLen) {
      next += dashLen;
    } else if (underscoreLen) {
      next += underscoreLen;
    }
    return { next, ok: true };
  }
  const arbitrary = consumeArbitraryToolMarkupNamePrefix(raw, lower, idx);
  if (arbitrary.ok) {
    return arbitrary;
  }
  return { next: idx, ok: false };
}

function consumeArbitraryToolMarkupNamePrefix(raw, _lower, idx) {
  const first = consumeToolMarkupPrefixSegment(raw, idx);
  if (!first.ok) {
    return { next: idx, ok: false };
  }
  let j = first.next;
  while (j < raw.length) {
    const segment = consumeToolMarkupPrefixSegment(raw, j);
    if (!segment.ok) {
      break;
    }
    j = segment.next;
  }
  let k = j;
  while (true) {
    const spacingLen = toolMarkupWhitespaceLikeLenAt(raw, k);
    if (!spacingLen) {
      break;
    }
    k += spacingLen;
  }
  let next = k;
  let ok = false;
  const sep = consumeToolMarkupSeparator(raw, next);
  if (sep.ok) {
    next = sep.next;
    ok = true;
  } else {
    const dashLen = toolMarkupDashLenAt(raw, next);
    const underscoreLen = toolMarkupUnderscoreLenAt(raw, next);
    if (dashLen) {
      next += dashLen;
      ok = true;
    } else if (underscoreLen) {
      next += underscoreLen;
      ok = true;
    }
  }
  if (!ok) {
    return { next: idx, ok: false };
  }
  while (true) {
    const spacingLen = toolMarkupWhitespaceLikeLenAt(raw, next);
    if (!spacingLen) {
      break;
    }
    next += spacingLen;
  }
  if (!hasToolMarkupNamePrefix(raw, next)) {
    return { next: idx, ok: false };
  }
  return { next, ok: true };
}

function consumeToolMarkupPrefixSegment(raw, idx) {
  if (idx < 0 || idx >= raw.length) {
    return { next: idx, ok: false };
  }
  const normalized = normalizeFullwidthASCIIChar(raw[idx]);
  if (/^[A-Za-z0-9]$/.test(normalized)) {
    return { next: idx + 1, ok: true };
  }
  return { next: idx, ok: false };
}

function hasToolMarkupNamePrefix(raw, start) {
  for (const name of TOOL_MARKUP_NAMES) {
    if (consumeToolKeyword(raw, start, name.raw).ok) {
      return true;
    }
    if (hasConfusablePartialKeywordPrefix(raw, start, name.raw)) {
      return true;
    }
  }
  return false;
}

function hasConfusablePartialKeywordPrefix(raw, start, keyword) {
  if (start < 0 || start >= raw.length) {
    return false;
  }
  let idx = start;
  let matched = 0;
  while (matched < keyword.length && idx < raw.length) {
    idx = skipToolMarkupIgnorables(raw, idx);
    if (idx >= raw.length) {
      break;
    }
    const expected = keyword[matched];
    if (expected === '_') {
      const underscoreLen = toolMarkupUnderscoreLenAt(raw, idx);
      if (!underscoreLen) {
        return false;
      }
      idx += underscoreLen;
      matched += 1;
      continue;
    }
    if (expected === '-') {
      const dashLen = toolMarkupDashLenAt(raw, idx);
      if (!dashLen) {
        return false;
      }
      idx += dashLen;
      matched += 1;
      continue;
    }
    const cp = raw.codePointAt(idx);
    const ch = String.fromCodePoint(cp);
    const folded = foldToolKeywordRune(ch);
    if (!folded || folded !== expected.toLowerCase()) {
      return false;
    }
    idx += ch.length;
    matched += 1;
  }
  return matched > 0 && matched < keyword.length && idx === raw.length;
}

function matchToolMarkupName(raw, start, epseLike) {
  for (const name of TOOL_MARKUP_NAMES) {
    if (name.epseOnly && !epseLike) {
      continue;
    }
    const matched = consumeToolKeyword(raw, start, name.raw);
    if (matched.ok) {
      return { name: name.canonical, len: matched.next - start };
    }
  }
  return { name: '', len: 0 };
}

function rawNameForTag(tag) {
  for (const candidate of TOOL_MARKUP_NAMES) {
    if (candidate.canonical === tag.name) {
      return candidate.raw;
    }
  }
  return tag.name || '';
}

module.exports = {
  consumeToolMarkupNamePrefix,
  consumeToolMarkupNamePrefixOnce,
  hasEPSENamePrefixOrPartial,
  hasPartialToolMarkupNameAfterArbitraryPrefix,
  hasToolMarkupNamePrefix,
  matchToolMarkupName,
  matchToolMarkupNameAfterArbitraryPrefix,
  rawNameForTag,
  toolMarkupPrefixContainsSlash,
};
