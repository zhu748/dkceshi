'use strict';

// Stateful tool-call marker normalizer. Mirrors internal/toolcall/marker.go
// (MarkerNormalizer / NormalizeMarkerText) so the Node stream mirror and the Go
// runtime agree byte-for-byte on how the per-API-key marker is folded back.
//
// The upstream model is instructed to use a per-caller marker instead of the
// canonical `EPSE` keyword, so the shared keyword never reaches it. Every
// consumer of the model output (tool sieve, standalone tool-call parser,
// repeat-loop guard) still speaks `EPSE`, so the marker has to be rewritten back
// before any of them run. Only tag prefixes outside CDATA are rewritten;
// ordinary occurrences in paths, source code and other values stay literal.
//
// SSE deltas can split the marker at any byte (`<|Q7Z` in one chunk and
// `K3M|tool_calls>` in the next), so the rewrite is stateful: a trailing
// fragment that could still grow into the marker is withheld until the next
// write (or flush) instead of being forwarded half-rewritten.

const EPSE_KEYWORD = 'EPSE';

function normalizeMarker(marker) {
  const text = asString(marker).toUpperCase();
  if (!text || text === EPSE_KEYWORD) {
    return '';
  }
  return text;
}

// createMarkerNormalizer builds a normalizer for marker. An empty or canonical
// ("EPSE") marker yields an inert normalizer, which is the legacy behaviour.
function createMarkerNormalizer(marker) {
  const active = normalizeMarker(marker);
  let buf = '';
  let tagPrefix = false;
  let inCDATA = false;
  let cdataOpen = 0;
  let cdataClose = 0;
  if (!active) {
    return {
      active: false,
      write(chunk) {
        return typeof chunk === 'string' ? chunk : '';
      },
      flush() {
        return '';
      },
    };
  }
  function normalizeRune(ch) {
    const code = ch.codePointAt(0);
    if (code >= 0xff01 && code <= 0xff5e) return String.fromCodePoint(code - 0xfee0);
    return ({ '〈': '<', '〉': '>', '“': '"', '”': '"', '‘': "'", '’': "'" })[ch] || ch;
  }
  function observeRune(original) {
    const ch = normalizeRune(original).toLowerCase();
    if (inCDATA) {
      if (ch === ']') cdataClose = Math.min(2, cdataClose + 1);
      else {
        if (ch === '>' && cdataClose === 2) inCDATA = false;
        cdataClose = 0;
      }
      tagPrefix = false;
      return;
    }
    const opener = '<![cdata[';
    if (ch === opener[cdataOpen]) {
      cdataOpen++;
      if (cdataOpen === opener.length) { inCDATA = true; cdataOpen = 0; }
    } else cdataOpen = ch === '<' ? 1 : 0;
    const separator = !/[a-z0-9\s<>/=\["']/i.test(ch) && ch !== '▁';
    if (ch === '<') tagPrefix = true;
    else if (!tagPrefix || (!['/', '∕', '⁄', '⧸'].includes(ch) && !separator)) tagPrefix = false;
    if (inCDATA) tagPrefix = false;
  }
  function rewrite(text, final) {
    let out = '';
    for (let i = 0; i < text.length;) {
      if (!inCDATA && tagPrefix && equalsFold(text.slice(i, i + active.length), active)) {
        const after = i + active.length;
        if (after === text.length && !final) return [out, text.slice(i)];
        if (after === text.length || !/[a-z0-9]/i.test(normalizeRune(String.fromCodePoint(text.codePointAt(after))))) {
          out += EPSE_KEYWORD;
          for (const ch of text.slice(i, after)) observeRune(ch);
          i = after;
          continue;
        }
      }
      const code = text.codePointAt(i);
      if (!final && code >= 0xd800 && code <= 0xdbff && i + 1 === text.length) return [out, text.slice(i)];
      const ch = String.fromCodePoint(code);
      out += ch;
      observeRune(ch);
      i += ch.length;
    }
    return [out, ''];
  }
  return {
    active: true,
    // write consumes the next stream chunk and returns the text that is safe to
    // forward to downstream consumers. A trailing fragment that could still grow
    // into the marker is withheld until the following write (or flush).
    //
    // The hold is measured on the text AFTER the last complete occurrence rather
    // than on the whole buffer: a complete occurrence always ends in the marker
    // itself, so measuring the whole buffer would let the occurrence's own tail
    // pass for the head of the next occurrence whenever the marker overlaps
    // itself ('N3KZ5N'), splitting the occurrence and leaking its head
    // un-rewritten. Anchoring the hold after the last occurrence keeps every
    // complete occurrence intact for the context-aware rewriter, which checks
    // the tag prefix and waits for the following boundary if needed.
    write(chunk) {
      const data = buf + (typeof chunk === 'string' ? chunk : '');
      buf = '';
      const hold = trailingMarkerPrefixLen(data.slice(lastMarkerOccurrenceEnd(data, active)), active);
      buf = data.slice(data.length - hold);
      const [out, tail] = rewrite(data.slice(0, data.length - hold), false);
      buf = tail + buf;
      return out;
    },
    // flush releases any withheld fragment at end of stream.
    flush() {
      const [out] = rewrite(buf, true);
      buf = '';
      return out;
    },
  };
}

// lastMarkerOccurrenceEnd returns the offset just past the last complete marker
// occurrence in data, or 0 when data carries no complete occurrence. It uses the
// left-to-right greedy scan (a match consumes marker.length characters).
// Partial-suffix buffering must never split the final complete occurrence.
function lastMarkerOccurrenceEnd(data, marker) {
  if (!marker || data.length < marker.length) {
    return 0;
  }
  let end = 0;
  let i = 0;
  while (i + marker.length <= data.length) {
    if (equalsFold(data.slice(i, i + marker.length), marker)) {
      i += marker.length;
      end = i;
      continue;
    }
    i += 1;
  }
  return end;
}

// trailingMarkerPrefixLen returns the length of the longest suffix of data that
// is a proper prefix of marker (case-insensitive ASCII), capped at
// len(marker) - 1.
//
// The scan is occurrence-agnostic: for a self-overlapping marker the tail of a
// complete occurrence is itself a proper prefix of the marker. Callers must
// therefore pass the text that FOLLOWS the last complete occurrence (see
// createMarkerNormalizer's write) so that a complete marker is never split.
function trailingMarkerPrefixLen(data, marker) {
  const max = Math.min(marker.length - 1, data.length);
  for (let l = max; l > 0; l -= 1) {
    if (equalsFold(data.slice(data.length - l), marker.slice(0, l))) {
      return l;
    }
  }
  return 0;
}

// Normalize protocol prefixes while preserving literal text and CDATA values.
function replaceMarkerKeyword(text, marker) {
  const normalizer = createMarkerNormalizer(marker);
  return normalizer.write(text) + normalizer.flush();
}

function equalsFold(a, b) {
  return a.length === b.length && a.toLowerCase() === b.toLowerCase();
}

function asString(v) {
  if (typeof v === 'string') {
    return v.trim();
  }
  if (v == null) {
    return '';
  }
  return String(v).trim();
}

module.exports = {
  EPSE_KEYWORD,
  createMarkerNormalizer,
  replaceMarkerKeyword,
};
