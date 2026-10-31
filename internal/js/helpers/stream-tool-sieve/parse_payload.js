'use strict';

// Public payload parser facade and tool-call assembly.
const { toRawString } = require('./state');
const {
  repairSwallowedCDATARanges,
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
  let raw = normalized.text.trim();
  if (!raw) {
    return [];
  }
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
    for (const block of findXmlElementBlocks(body, 'invoke')) {
      const parsed = parseMarkupSingleToolCall(block);
      if (parsed) {
        out.push(parsed);
      }
    }
  }
  return out;
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
  const name = toRawString(attrs.name).trim();
  if (!name) {
    return null;
  }
  const inner = toRawString(block.body).trim();

  if (inner) {
    try {
      const decoded = JSON.parse(inner);
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
  let prev = 0;
  for (const match of findXmlElementBlocks(inner, 'parameter')) {
    if (hasUnconsumedParameterMarkup(inner.slice(prev, match.start))) return null;
    const parameterAttrs = parseTagAttributes(match.attrs);
    const paramName = toRawString(parameterAttrs.name).trim();
    if (!paramName) {
      return null;
    }
    appendMarkupValue(input, paramName, parseMarkupValue(match.body, paramName));
    prev = match.end;
  }
  if (hasUnconsumedParameterMarkup(inner.slice(prev))) return null;
  if (Object.keys(input).length === 0 && inner.trim() !== '') {
    return null;
  }
  return { name, input };
}

function hasUnconsumedParameterMarkup(text) {
  for (let i = 0; i < text.length;) {
    const skipped = skipXmlIgnoredSection(text, i);
    if (skipped.blocked) return false;
    if (skipped.advanced) { i = skipped.next; continue; }
    const span = markdownCodeSpanEnd(text, i);
    if (span.ok) { i = span.end; continue; }
    const tag = scanToolMarkupTagAt(text, i);
    if (tag && tag.name === 'parameter' && !tag.closing) return true;
    const prefix = '<parameter';
    if (text.slice(i, i + prefix.length).toLowerCase() === prefix && hasXmlTagBoundary(text, i + prefix.length)) return true;
    i += 1;
  }
  return false;
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
  repairSwallowedCDATARanges,
  sanitizeLooseCDATA,
  stripDSMLResidue,
  stripFencedCodeBlocks,
  stripMarkdownCodeSpans,
};
