'use strict';

// Recovery of loose or swallowed CDATA element boundaries.
const { toRawString } = require('./state');
const {
  findToolCDATAEnd,
  indexToolCDATAOpen,
  toolCDATACloseLenAt,
  toolCDATAOpenLenAt,
} = require('./cdata_scan');
const {
  findToolMarkupTagOutsideIgnored,
} = require('./markup_scan');
const {
  consumeToolKeyword,
  consumeToolMarkupClosingSlash,
  consumeToolMarkupLessThan,
  consumeToolMarkupSeparator,
  hasXmlTagBoundary,
  normalizeFullwidthASCIIChar,
  xmlTagStartDelimiterLenAt,
} = require('./markup_syntax');

function sanitizeLooseCDATA(text) {
  const raw = toRawString(text);
  if (!raw) {
    return '';
  }

  let out = '';
  let pos = 0;
  let changed = false;
  while (pos < raw.length) {
    const start = indexToolCDATAOpen(raw, pos);
    if (start < 0) {
      out += raw.slice(pos);
      break;
    }
    const openLen = toolCDATAOpenLenAt(raw, start);
    const contentStart = start + openLen;
    out += raw.slice(pos, start);

    const endRel = findToolCDATAEnd(raw, contentStart);
    if (endRel >= 0) {
      const end = endRel + toolCDATACloseLenAt(raw, endRel);
      out += raw.slice(start, end);
      pos = end;
      continue;
    }

    changed = true;
    out += raw.slice(contentStart);
    pos = raw.length;
  }

  return changed ? out : raw;
}

// Go parity: internal/toolcall/toolcalls_cdata_boundary_repair.go
//
// findToolCDATAEnd searches forward without an element bound, so a `<![CDATA[`
// whose own `]]>` is missing keeps scanning until the next `]]>` anywhere in the
// text — which belongs to a LATER parameter. The value then silently absorbs
// `</|EPSE|parameter><|EPSE|parameter name="...">` plus the next parameter's
// body, and that later parameter disappears from the parsed input.
//
// The repair below closes such a range at its own element boundary. It is
// deliberately narrow: the greedy interpretation stays authoritative and is only
// replaced when the boundary interpretation parses into strictly more tool calls
// / parameters (see boundaryRepairOutranks in parse.js).
const TOOL_MARKUP_STRUCTURAL_TAG_PREFIXES = [
  '<tool_calls', '</tool_calls',
  '<tool-calls', '</tool-calls',
  '<toolcalls', '</toolcalls',
  '<invoke', '</invoke',
  '<parameter', '</parameter',
];

// nextToolMarkupStructuralBoundary returns the offset of the first tool-markup
// structural tag at or after from, or -1 when there is none.
//
// Unlike every other scanner here it deliberately does NOT skip CDATA regions:
// it is called from inside a CDATA body precisely to find where the enclosing
// element ends when that body has no `]]>` of its own.
function nextToolMarkupStructuralBoundary(text, from) {
  const raw = toRawString(text);
  const lower = raw.toLowerCase();
  for (let i = Math.max(0, from || 0); i < raw.length; i += 1) {
    // Cheap guard: every candidate starts with a `<` (or a fullwidth/CJK
    // variant). Bodies are routinely tens of KB, so the expensive matchers must
    // not run on every character.
    if (!xmlTagStartDelimiterLenAt(raw, i)) {
      continue;
    }
    if (detectEPSETagAt(raw, i)) {
      return i;
    }
    for (const prefix of TOOL_MARKUP_STRUCTURAL_TAG_PREFIXES) {
      if (lower.startsWith(prefix, i) && hasXmlTagBoundary(raw, i + prefix.length)) {
        return i;
      }
    }
  }
  return -1;
}

// detectEPSETagAt reports whether an EPSE-prefixed markup tag begins at start,
// ignoring the local name, so `<|EPSE|parameter>`, `</|EPSE>` and `<EPSE ...>`
// variants all match.
function detectEPSETagAt(text, start) {
  const raw = toRawString(text);
  const lt = consumeToolMarkupLessThan(raw, start);
  if (!lt.ok) {
    return false;
  }
  let i = lt.next;
  while (true) {
    const next = consumeToolMarkupLessThan(raw, i);
    if (!next.ok) {
      break;
    }
    i = next.next;
  }
  i = consumeToolMarkupClosingSlash(raw, i).next;
  while (true) {
    const sep = consumeToolMarkupSeparator(raw, i);
    if (!sep.ok) {
      break;
    }
    i = sep.next;
  }
  const epse = consumeToolKeyword(raw, i, 'epse');
  if (!epse.ok) {
    return false;
  }
  // Guard against words that merely start with "epse" (e.g. "epsentence").
  const tail = normalizeFullwidthASCIIChar(raw[epse.next] || '');
  return !/^[A-Za-z0-9]$/.test(tail || '');
}

// scanClosedCDATARanges records every well-closed CDATA block, skipping an
// unclosed opener and continuing instead of blocking the remainder the way
// skipXmlIgnoredSection does.
function scanClosedCDATARanges(text) {
  const raw = toRawString(text);
  const ranges = [];
  let pos = 0;
  while (pos < raw.length) {
    const start = indexToolCDATAOpen(raw, pos);
    if (start < 0) {
      break;
    }
    const contentStart = start + toolCDATAOpenLenAt(raw, start);
    const contentEnd = findToolCDATAEnd(raw, contentStart);
    if (contentEnd < 0) {
      pos = contentStart;
      continue;
    }
    const closeLen = toolCDATACloseLenAt(raw, contentEnd);
    ranges.push({ contentStart, contentEnd, closeLen });
    pos = contentEnd + closeLen;
  }
  return ranges;
}

// boundedCDATAContentEnd splits an element-bounded CDATA region into its content
// and its close marker. A trailing `]]` (the `]]>` the model forgot the `>` on)
// is kept out of the content so the restored value carries no stray suffix.
function boundedCDATAContentEnd(text, contentStart, limit) {
  const raw = toRawString(text);
  const endRel = findToolCDATAEnd(raw.slice(0, limit), contentStart);
  if (endRel >= 0) {
    return { contentEnd: endRel, closeLen: toolCDATACloseLenAt(raw, endRel) };
  }
  let end = limit;
  while (end > contentStart && /\s/u.test(raw[end - 1])) {
    end -= 1;
  }
  if (end - contentStart >= 2 && raw.slice(end - 2, end) === ']]') {
    return { contentEnd: end - 2, closeLen: 2 };
  }
  return { contentEnd: limit, closeLen: 0 };
}

// cdataRangeSwallowedNextParameter reports whether a greedy CDATA range consumed
// the close marker of a later parameter.
//
// The signature has two halves, and both are required:
//
//   - an *unbalanced* CDATA opener inside the body — an opener with no `]]>` of
//     its own before the body ends. Only possible when the body ate a later
//     parameter's opener and terminated on that parameter's close marker.
//   - the range's own `</parameter>` close tag sitting inside the body, before
//     that opener. This is what distinguishes "escaped my element" from "quotes
//     tool-call markup".
//
// The second half keeps a legitimately quoted example intact: a value embedding a
// tool-call sample makes the greedy scan stop at the sample's inner `]]>`, so the
// body holds the sample's opening parameter tag and an unbalanced opener — but
// never a close tag preceding them, because the value never left its element.
function cdataRangeSwallowedNextParameter(text, range) {
  const body = toRawString(text).slice(range.contentStart, range.contentEnd);
  let pos = 0;
  while (pos < body.length) {
    const open = indexToolCDATAOpen(body, pos);
    if (open < 0) {
      return false;
    }
    const contentStart = open + toolCDATAOpenLenAt(body, open);
    const closeIdx = indexToolCDATAClose(body, contentStart);
    if (closeIdx < 0) {
      return cdataOpenerEscapedOwnElement(body, open);
    }
    pos = closeIdx + toolCDATACloseLenAt(body, closeIdx);
  }
  return false;
}

function indexToolCDATAClose(text, from) {
  const raw = toRawString(text);
  for (let i = Math.max(0, from || 0); i < raw.length; i += 1) {
    if (toolCDATACloseLenAt(raw, i)) {
      return i;
    }
  }
  return -1;
}

// cdataOpenerEscapedOwnElement reports whether the unbalanced CDATA opener at
// open is preceded by the tag sequence that can only mean the body ran out of its
// own element and into the next parameter: a closing `parameter` tag somewhere
// earlier, and an opening `parameter` tag immediately before the opener.
function cdataOpenerEscapedOwnElement(body, open) {
  if (open <= 0 || open > body.length) {
    return false;
  }
  const head = body.slice(0, open).replace(/[ \t\r\n]+$/u, '');
  if (!head) {
    return false;
  }
  let sawOwnClose = false;
  let last = null;
  for (let from = 0; from < head.length;) {
    const tag = findToolMarkupTagOutsideIgnored(head, from);
    if (!tag) {
      break;
    }
    if (tag.closing && tag.name === 'parameter') {
      sawOwnClose = true;
    }
    last = tag;
    from = tag.end + 1;
  }
  if (!sawOwnClose || !last) {
    return false;
  }
  return !last.closing && last.name === 'parameter' && last.end === head.length - 1;
}

// repairSwallowedCDATARanges closes every greedy CDATA range that swallowed a
// later parameter at its own element boundary, leaving all other text — and all
// other CDATA ranges — byte-identical.
//
// Releasing a swallowed tail can expose a further unclosed parameter inside it
// (two consecutive parameters both missing their `]]>` present as a single greedy
// range), so the pass repeats to a fixpoint, bounded by the number of CDATA
// openers since every pass closes at least one.
function repairSwallowedCDATARanges(text) {
  let repaired = toRawString(text);
  for (let budget = countToolCDATAOpeners(repaired); budget > 0; budget -= 1) {
    const next = repairSwallowedCDATARangesOnce(repaired);
    if (next === repaired) {
      break;
    }
    repaired = next;
  }
  return repaired;
}

function countToolCDATAOpeners(text) {
  const raw = toRawString(text);
  let count = 0;
  let pos = 0;
  while (pos < raw.length) {
    const open = indexToolCDATAOpen(raw, pos);
    if (open < 0) {
      break;
    }
    count += 1;
    pos = open + toolCDATAOpenLenAt(raw, open);
  }
  return count;
}

function repairSwallowedCDATARangesOnce(text) {
  const raw = toRawString(text);
  if (!raw) {
    return raw;
  }
  let out = '';
  let prev = 0;
  let changed = false;
  for (const range of scanClosedCDATARanges(raw)) {
    if (range.contentStart < prev || !cdataRangeSwallowedNextParameter(raw, range)) {
      continue;
    }
    const limit = nextToolMarkupStructuralBoundary(raw, range.contentStart);
    if (limit < 0 || limit > range.contentEnd) {
      continue;
    }
    const bounded = boundedCDATAContentEnd(raw, range.contentStart, limit);
    if (bounded.contentEnd <= range.contentStart || bounded.contentEnd + bounded.closeLen > raw.length) {
      continue;
    }
    changed = true;
    out += raw.slice(prev, bounded.contentEnd);
    out += ']]>';
    prev = bounded.contentEnd + bounded.closeLen;
  }
  if (!changed) {
    return raw;
  }
  return out + raw.slice(prev);
}

module.exports = {
  repairSwallowedCDATARanges,
  sanitizeLooseCDATA,
};
