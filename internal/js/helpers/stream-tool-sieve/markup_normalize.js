'use strict';

// Ignore literal examples and canonicalize recognized tool wrappers.
const { toRawString } = require('./state');
const {
  indexToolCDATAOpen,
  skipXmlIgnoredSection,
  toolCDATACloseLenAt,
  toolCDATAOpenLenAt,
} = require('./cdata_scan');
const {
  isFenceCloseLine,
  markdownCodeSpanEnd,
  parseFenceOpenLine,
} = require('./markdown_fence');
const {
  parseCanonicalToolMarkupAttrs,
  quoteCanonicalXMLAttrValue,
} = require('./markup_attributes');
const {
  consumeToolMarkupNamePrefix,
  rawNameForTag,
} = require('./markup_names');
const {
  scanToolMarkupTagAt,
} = require('./markup_scan');
const {
  consumeToolKeyword,
  consumeToolMarkupClosingSlash,
  skipToolMarkupIgnorables,
  xmlTagStartDelimiterLenAt,
} = require('./markup_syntax');

// Keep the original coordinate system throughout scanning and repair. Only
// semantic fields (tool names, attribute keys, JSON literals) trim explicitly.

function stripFencedCodeBlocks(text) {
  const t = typeof text === 'string' ? text : '';
  if (!t) {
    return '';
  }
  const lines = t.split('\n');
  const out = [];
  let inFence = false;
  let fenceChar = '';
  let fenceLen = 0;
  let inCDATA = false;
  let beforeFenceIdx = 0;
  // Byte offset in the original text where the current unclosed fence began,
  // so its raw tail can be recovered when the fence is a false positive.
  let fenceStartOffset = -1;
  let lineStart = 0;

  for (let li = 0; li < lines.length; li += 1) {
    const line = lines[li];
    const lineWithNL = li < lines.length - 1 ? line + '\n' : line;
    const curLineStart = lineStart;
    lineStart += lineWithNL.length;

    // CDATA protection
    if (inCDATA || cdataStartsBeforeFence(line)) {
      out.push(lineWithNL);
      inCDATA = updateCDATAStateLine(inCDATA, line);
      continue;
    }

    const trimmed = line.replace(/^[ \t]+/, '');
    if (!inFence) {
      const fence = parseFenceOpenLine(trimmed);
      if (fence) {
        inFence = true;
        fenceChar = fence.ch;
        fenceLen = fence.count;
        beforeFenceIdx = out.length;
        fenceStartOffset = curLineStart;
        continue;
      }
      out.push(lineWithNL);
      continue;
    }

    if (isFenceCloseLine(trimmed, fenceChar, fenceLen)) {
      inFence = false;
      fenceChar = '';
      fenceLen = 0;
    }
  }

  if (inFence) {
    // Unclosed fence: normally drop the fenced region and keep only the
    // content collected before it. But tool-call parameters routinely carry
    // an odd number of markdown fences (```), which makes a real tool call
    // look like an unclosed fence and swallows the trailing tool close tags.
    // If the dropped tail still carries tool-structural closing tags, treat
    // the fence as a false positive and preserve the raw tail so the close
    // tags survive.
    const head = beforeFenceIdx > 0 ? out.slice(0, beforeFenceIdx).join('') : '';
    if (fenceStartOffset >= 0 && fenceStartOffset <= t.length) {
      const tail = t.slice(fenceStartOffset);
      if (textContainsToolMarkupCloseTag(tail)) {
        return head + tail;
      }
    }
    return head;
  }
  return out.join('');
}

// textContainsToolMarkupCloseTag reports whether text carries a closing tool
// markup tag (EPSE or canonical) for tool_calls / invoke / parameter. It
// deliberately does NOT skip markdown fences: used to decide whether an
// apparently-unclosed fence actually straddles a real tool call whose closing
// tags must not be discarded.
function textContainsToolMarkupCloseTag(text) {
  const raw = toRawString(text);
  for (let i = 0; i < raw.length; i += 1) {
    const tag = scanToolMarkupTagAt(raw, i);
    if (tag && tag.closing) {
      return true;
    }
  }
  return false;
}

function stripMarkdownCodeSpans(text) {
  const raw = toRawString(text);
  if (!raw) {
    return '';
  }
  let out = '';
  for (let i = 0; i < raw.length;) {
    const skipped = skipXmlIgnoredSection(raw, i);
    if (skipped.blocked) {
      out += raw.slice(i);
      break;
    }
    if (skipped.advanced) {
      out += raw.slice(i, skipped.next);
      i = skipped.next;
      continue;
    }
    const spanEnd = markdownCodeSpanEnd(raw, i);
    if (spanEnd.ok) {
      i = spanEnd.end;
      continue;
    }
    out += raw[i];
    i += 1;
  }
  return out;
}

function cdataStartsBeforeFence(line) {
  const cdataIdx = indexToolCDATAOpen(line, 0);
  if (cdataIdx < 0) return false;
  const fenceIdx = Math.min(
    line.indexOf('```') >= 0 ? line.indexOf('```') : Infinity,
    line.indexOf('~~~') >= 0 ? line.indexOf('~~~') : Infinity,
  );
  return fenceIdx === Infinity || cdataIdx < fenceIdx;
}

function updateCDATAStateLine(inCDATA, line) {
  let pos = 0;
  let state = inCDATA;
  while (pos < line.length) {
    if (state) {
      let end = -1;
      let closeLen = 0;
      for (let i = pos; i < line.length; i += 1) {
        const foundLen = toolCDATACloseLenAt(line, i);
        if (foundLen > 0) {
          end = i;
          closeLen = foundLen;
          break;
        }
      }
      if (end < 0) return true;
      pos = end + closeLen;
      state = false;
      continue;
    }
    const start = indexToolCDATAOpen(line, pos);
    if (start < 0) return false;
    pos = start + toolCDATAOpenLenAt(line, start);
    state = true;
  }
  return state;
}

function normalizeEPSEToolCallMarkup(text) {
  const raw = toRawString(text);
  if (!raw) {
    return { text: '', ok: true };
  }
  const canonicalized = canonicalizeToolCallCandidateSpans(raw);
  const styles = containsToolMarkupSyntaxOutsideIgnored(canonicalized);
  if (!styles.epse && !styles.canonical) {
    return { text: canonicalized, ok: true };
  }
  return {
    text: replaceEPSEToolMarkupOutsideIgnored(canonicalized),
    ok: true,
  };
}

function containsToolCallWrapperSyntaxOutsideIgnored(text) {
  const raw = toRawString(text);
  const styles = { epse: false, canonical: false };
  if (!raw) {
    return styles;
  }
  for (let i = 0; i < raw.length;) {
    const skipped = skipXmlIgnoredSection(raw, i);
    if (skipped.blocked) {
      return styles;
    }
    if (skipped.advanced) {
      i = skipped.next;
      continue;
    }
    const spanEnd = markdownCodeSpanEnd(raw, i);
    if (spanEnd.ok) {
      i = spanEnd.end;
      continue;
    }
    const tag = scanToolMarkupTagAt(raw, i);
    if (tag) {
      if (tag.name !== 'tool_calls') {
        i = tag.end + 1;
        continue;
      }
      if (tag.epseLike) {
        styles.epse = true;
      } else {
        styles.canonical = true;
      }
      if (styles.epse && styles.canonical) {
        return styles;
      }
      i = tag.end + 1;
      continue;
    }
    i += 1;
  }
  return styles;
}

function containsToolMarkupSyntaxOutsideIgnored(text) {
  const raw = toRawString(text);
  const styles = { epse: false, canonical: false };
  if (!raw) {
    return styles;
  }
  for (let i = 0; i < raw.length;) {
    const skipped = skipXmlIgnoredSection(raw, i);
    if (skipped.blocked) {
      return styles;
    }
    if (skipped.advanced) {
      i = skipped.next;
      continue;
    }
    const spanEnd = markdownCodeSpanEnd(raw, i);
    if (spanEnd.ok) {
      i = spanEnd.end;
      continue;
    }
    const tag = scanToolMarkupTagAt(raw, i);
    if (tag) {
      if (tag.epseLike) {
        styles.epse = true;
      } else {
        styles.canonical = true;
      }
      if (styles.epse && styles.canonical) {
        return styles;
      }
      i = tag.end + 1;
      continue;
    }
    i += 1;
  }
  return styles;
}

function replaceEPSEToolMarkupOutsideIgnored(text) {
  const raw = toRawString(text);
  if (!raw) {
    return '';
  }
  let out = '';
  for (let i = 0; i < raw.length;) {
    const skipped = skipXmlIgnoredSection(raw, i);
    if (skipped.blocked) {
      out += raw.slice(i);
      break;
    }
    if (skipped.advanced) {
      out += raw.slice(i, skipped.next);
      i = skipped.next;
      continue;
    }
    const spanEnd = markdownCodeSpanEnd(raw, i);
    if (spanEnd.ok) {
      out += raw.slice(i, spanEnd.end);
      i = spanEnd.end;
      continue;
    }
    const tag = scanToolMarkupTagAt(raw, i);
    if (tag) {
      const attrs = tag.parameterName ? ` name="${tag.parameterName}"` : raw.slice(tag.nameEnd, tag.end);
      out += `<${tag.closing ? '/' : ''}${tag.name}${attrs}>`;
      i = tag.end + 1;
      continue;
    }
    out += raw[i];
    i += 1;
  }
  return out;
}

function canonicalizeToolCallCandidateSpans(text) {
  const raw = toRawString(text);
  if (!raw) {
    return '';
  }
  let out = '';
  for (let i = 0; i < raw.length;) {
    const skipped = skipXmlIgnoredSection(raw, i);
    if (skipped.blocked) {
      out += raw.slice(i);
      break;
    }
    if (skipped.advanced) {
      out += raw.slice(i, skipped.next);
      i = skipped.next;
      continue;
    }
    const spanEnd = markdownCodeSpanEnd(raw, i);
    if (spanEnd.ok) {
      out += raw.slice(i, spanEnd.end);
      i = spanEnd.end;
      continue;
    }
    const tag = scanToolMarkupTagAt(raw, i);
    if (!tag) {
      out += raw[i];
      i += 1;
      continue;
    }
    out += canonicalizeRecognizedToolMarkupTag(raw.slice(tag.start, tag.end + 1), tag);
    i = tag.end + 1;
  }
  return out;
}

function canonicalizeRecognizedToolMarkupTag(rawTag, tag) {
  const raw = toRawString(rawTag);
  if (!raw || !tag) {
    return raw;
  }
  let idx = 0;
  const startLen = xmlTagStartDelimiterLenAt(raw, idx);
  if (startLen > 0) {
    idx += startLen;
  }
  while (idx < raw.length) {
    idx = skipToolMarkupIgnorables(raw, idx);
    const delimLen = xmlTagStartDelimiterLenAt(raw, idx);
    if (!delimLen) {
      break;
    }
    idx += delimLen;
  }
  idx = skipToolMarkupIgnorables(raw, idx);
  if (tag.closing) {
    const slash = consumeToolMarkupClosingSlash(raw, idx);
    if (slash.closing) {
      idx = slash.next;
    }
  }
  const prefix = consumeToolMarkupNamePrefix(raw, raw.toLowerCase(), idx);
  idx = prefix.next;
  const nameMatch = consumeToolKeyword(raw, idx, rawNameForTag(tag));
  const afterName = nameMatch.ok ? nameMatch.next : idx;
  const attrs = tag.parameterName
    ? [{ key: 'name', value: tag.parameterName }]
    : parseCanonicalToolMarkupAttrs(raw, afterName);

  let out = '<';
  if (tag.closing) {
    out += '/';
  }
  if (tag.epseLike) {
    out += '|EPSE|';
  }
  out += tag.name;
  for (const attr of attrs) {
    if (!attr || !attr.key) {
      continue;
    }
    out += ` ${attr.key}="${quoteCanonicalXMLAttrValue(attr.value)}"`;
  }
  if (tag.selfClosing) {
    out += '/';
  }
  out += '>';
  return out;
}

module.exports = {
  containsToolCallWrapperSyntaxOutsideIgnored,
  containsToolMarkupSyntaxOutsideIgnored,
  normalizeEPSEToolCallMarkup,
  stripFencedCodeBlocks,
  stripMarkdownCodeSpans,
};
