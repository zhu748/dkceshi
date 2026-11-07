'use strict';

// Public payload parser facade and tool-call assembly.
const { toRawString } = require('./state');
const { scanMarkupLimits } = require('./markup_budget');
const { parameterMarkupLimitExceeded } = require('./parameter_markup_budget');
const { unescapeHtml } = require('./entities');
const { parseJSONStrict, hasInvalidJSONLiteral } = require('./strict_json');
const { emptyToolMarkupGap } = require('./body_integrity');
const {
  sanitizeLooseCDATA,
} = require('./cdata_repair');
const {
  indexToolCDATAOpen,
  skipXmlIgnoredSection,
} = require('./cdata_scan');
const {
  stripDSMLResidue,
} = require('./dsml_residue');
const {
  markdownCodeSpanEnd,
} = require('./markdown_fence');
const {
  parseTagAttributes,
  parseCanonicalToolMarkupAttrs,
} = require('./markup_attributes');
const {
  findXmlElementBlocks,
} = require('./markup_elements');
const {
  containsToolCallWrapperSyntaxOutsideIgnored,
  containsToolMarkupSyntaxOutsideIgnored,
  normalizeEPSEToolCallMarkup,
  stripFencedCodeBlocks,
  stripMarkdownCodeSpans,
} = require('./markup_normalize');
const {
  findDSMLTagOutsideIgnored,
  findMatchingDSMLClose,
  findMatchingToolMarkupClose,
  findPartialToolMarkupStart,
  findToolMarkupTagOutsideIgnored,
  scanToolMarkupTagAt,
} = require('./markup_scan');
const {
  hasXmlTagBoundary,
  xmlTagEndDelimiterLenEndingAt,
} = require('./markup_syntax');
const {
  appendMarkupValue,
  parseMarkupValue,
} = require('./markup_value');

function parseMarkupToolCalls(text) {
  const normalized = normalizeEPSEToolCallMarkup(toRawString(text));
  if (!normalized.ok) {
    return [];
  }
  return parseNormalizedMarkupToolCalls(normalized.text);
}

// The public facade and recovery path already normalized this source. Reusing
// it avoids a second full canonicalization of every literal and tool batch.
function parseNormalizedMarkupToolCalls(text) {
  let raw = text.trim();
  if (!raw) {
    return [];
  }
  const checks = scanMarkupLimits(raw, unsupportedToolTagAttributes);
  if (checks.limited || checks.unsupported) return [];
  let wrappers = findToolCallElementBlocksOutsideIgnored(raw);
  if (wrappers.length === 0 && hasRepairableXMLToolCallsWrapper(raw)) {
    const repaired = repairMissingXMLToolCallsOpeningWrapper(raw);
    if (repaired !== raw) {
      raw = repaired;
      wrappers = findToolCallElementBlocksOutsideIgnored(raw);
    }
  }
  const out = [];
  for (const wrapper of wrappers) {
    const body = toRawString(wrapper.body);
    const calls = [];
    let prev = 0;
    let complete = true;
    for (const block of findXmlElementBlocks(body, 'invoke')) {
      const parsed = parseMarkupSingleToolCall(block);
      if (!parsed || !emptyToolMarkupGap(body.slice(prev, block.start))) { complete = false; break; }
      calls.push(parsed);
      prev = block.end;
    }
    if (complete && emptyToolMarkupGap(body.slice(prev))) out.push(...calls);
  }
  return out;
}

function unsupportedToolTagAttributes(text, tag) {
  const attrs = parseCanonicalToolMarkupAttrs(text.slice(tag.start, tag.end + 1), tag.nameEnd - tag.start);
  const seen = new Set();
  for (const attr of attrs) {
    const allowed = !tag.closing && (tag.name === 'invoke' && ['name', 'call_id'].includes(attr.key) || tag.name === 'parameter' && attr.key === 'name' || tag.name === 'tool_calls' && ['id', 'version'].includes(attr.key));
    if (!allowed || seen.has(attr.key)) return true;
    seen.add(attr.key);
  }
  return false;
}

function findToolCallElementBlocksOutsideIgnored(text) {
  const raw = toRawString(text);
  const out = [];
  for (let searchFrom = 0; searchFrom < raw.length;) {
    const tag = findToolMarkupTagOutsideIgnored(raw, searchFrom);
    if (!tag) {
      break;
    }
    if (tag.closing || tag.name !== 'tool_calls') {
      searchFrom = tag.end + 1;
      continue;
    }
    const closeTag = findMatchingToolMarkupClose(raw, tag);
    if (!closeTag) {
      searchFrom = tag.end + 1;
      continue;
    }
    const endDelim = xmlTagEndDelimiterLenEndingAt(raw, tag.end);
    const attrsEnd = endDelim > 0 ? tag.end + 1 - endDelim : tag.end + 1;
    out.push({
      attrs: raw.slice(tag.nameEnd, attrsEnd),
      body: raw.slice(tag.end + 1, closeTag.start),
      start: tag.start,
      end: closeTag.end + 1,
    });
    searchFrom = closeTag.end + 1;
  }
  return out;
}

function parseMarkupSingleToolCall(block) {
  const attrs = parseTagAttributes(block.attrs);
  const name = unescapeHtml(toRawString(attrs.name)).trim();
  if (!name) {
    return null;
  }
  const inner = toRawString(block.body).trim();

  if (inner) {
    try {
      const decoded = parseJSONStrict(inner);
      if (decoded && typeof decoded === 'object' && !Array.isArray(decoded)) {
        const key = Object.hasOwn(decoded, 'input') ? 'input' : Object.hasOwn(decoded, 'parameters') ? 'parameters' : null;
        if (!key) return Object.keys(decoded).length === 0 ? { name, input: {} } : null;
        if (Object.keys(decoded).length !== 1) return null;
        const input = decoded[key];
        if (!input || typeof input !== 'object' || Array.isArray(input)) return null;
        return { name, input };
      }
    } catch (_err) {
      // Not JSON, continue with markup parsing.
    }
  }
  const input = {};
  const counts = new Map();
  let prev = 0;
  for (const match of findXmlElementBlocks(inner, 'parameter')) {
    if (!emptyToolMarkupGap(inner.slice(prev, match.start))) return null;
    const parameterAttrs = parseTagAttributes(match.attrs);
    const paramName = unescapeHtml(toRawString(parameterAttrs.name));
    if (!Object.hasOwn(parameterAttrs, 'name')) {
      return null;
    }
    const body = toRawString(match.body).trim();
    if (hasInvalidJSONLiteral(body) || parameterMarkupLimitExceeded(body)) return null;
    appendMarkupValue(input, counts, paramName, parseMarkupValue(match.body, paramName));
    prev = match.end;
  }
  if (!emptyToolMarkupGap(inner.slice(prev))) return null;
  if (Object.keys(input).length === 0 && inner.trim() !== '') {
    return null;
  }
  return { name, input };
}

function hasRepairableXMLToolCallsWrapper(text) {
  const raw = toRawString(text).trim();
  if (!raw || firstToolMarkupTagByName(raw, 'tool_calls', false)) {
    return false;
  }
  const invoke = firstToolMarkupTagByName(raw, 'invoke', false);
  if (!invoke) {
    return false;
  }
  const close = lastToolMarkupTagByName(raw, 'tool_calls', true);
  if (!close) {
    return false;
  }
  return invoke.start < close.start;
}

function repairMissingXMLToolCallsOpeningWrapper(text) {
  const raw = toRawString(text);
  if (firstToolMarkupTagByName(raw, 'tool_calls', false)) {
    return raw;
  }
  const invoke = firstToolMarkupTagByName(raw, 'invoke', false);
  const close = lastToolMarkupTagByName(raw, 'tool_calls', true);
  if (!invoke || !close || invoke.start >= close.start) {
    return raw;
  }
  return `${raw.slice(0, invoke.start)}<tool_calls>${raw.slice(invoke.start, close.start)}</tool_calls>${raw.slice(close.end + 1)}`;
}

function firstToolMarkupTagByName(text, name, closing) {
  const raw = toRawString(text);
  for (let searchFrom = 0; searchFrom < raw.length;) {
    const tag = findToolMarkupTagOutsideIgnored(raw, searchFrom);
    if (!tag) {
      break;
    }
    if (tag.name === name && tag.closing === closing) {
      return tag;
    }
    searchFrom = tag.end + 1;
  }
  return null;
}

function lastToolMarkupTagByName(text, name, closing) {
  const raw = toRawString(text);
  let last = null;
  for (let searchFrom = 0; searchFrom < raw.length;) {
    const tag = findToolMarkupTagOutsideIgnored(raw, searchFrom);
    if (!tag) {
      break;
    }
    if (tag.name === name && tag.closing === closing) {
      last = tag;
    }
    searchFrom = tag.end + 1;
  }
  return last;
}

module.exports = {
  containsToolCallWrapperSyntaxOutsideIgnored,
  containsToolMarkupSyntaxOutsideIgnored,
  findDSMLTagOutsideIgnored,
  findMatchingDSMLClose,
  findMatchingToolMarkupClose,
  findPartialToolMarkupStart,
  findToolMarkupTagOutsideIgnored,
  hasRepairableXMLToolCallsWrapper,
  indexToolCDATAOpen,
  normalizeEPSEToolCallMarkup,
  parseMarkupToolCalls,
  parseNormalizedMarkupToolCalls,
  sanitizeLooseCDATA,
  stripDSMLResidue,
  stripFencedCodeBlocks,
  stripMarkdownCodeSpans,
};
