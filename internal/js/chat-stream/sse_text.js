'use strict';

// Content coercion, reference markers and leaked control-token cleanup.

const LEAKED_BOS_MARKER_PATTERN = /<[\|\uFF5C]\s*begin[_▁]of[_▁]sentence\s*[\|\uFF5C]>/gi;

const LEAKED_THOUGHT_MARKER_PATTERN = /<[\|\uFF5C]\s*(?:begin[_▁])?[_▁]*of[_▁]thought\s*[\|\uFF5C]>/gi;

const LEAKED_META_MARKER_PATTERN = /<[\|\uFF5C]\s*(?:assistant|tool|end[_▁]of[_▁]sentence|end[_▁]of[_▁]thinking|end[_▁]of[_▁]thought|end[_▁]of[_▁]toolresults|end[_▁]of[_▁]instructions)\s*[\|\uFF5C]>/gi;

function stripThinkTags(text) {
  if (typeof text !== 'string' || !text) {
    return text;
  }
  return text.replace(/<\/?\s*think\s*>/gi, '');
}

function asContentString(v, stripReferenceMarkers = true) {
  if (typeof v === 'string') {
    return stripReferenceMarkers ? stripReferenceMarkersText(v) : v;
  }
  if (Array.isArray(v)) {
    let out = '';
    for (const item of v) {
      out += asContentString(item, stripReferenceMarkers);
    }
    return out;
  }
  if (v && typeof v === 'object') {
    if (Object.prototype.hasOwnProperty.call(v, 'content')) {
      return asContentString(v.content, stripReferenceMarkers);
    }
    if (Object.prototype.hasOwnProperty.call(v, 'v')) {
      return asContentString(v.v, stripReferenceMarkers);
    }
    if (Object.prototype.hasOwnProperty.call(v, 'text')) {
      return asContentString(v.text, stripReferenceMarkers);
    }
    if (Object.prototype.hasOwnProperty.call(v, 'value')) {
      return asContentString(v.value, stripReferenceMarkers);
    }
    return '';
  }
  if (v == null) {
    return '';
  }
  const text = String(v);
  return stripReferenceMarkers ? stripReferenceMarkersText(text) : text;
}

function stripReferenceMarkersText(text) {
  if (!text) {
    return text;
  }
  return text
    .replace(/\[(?:citation|reference):\s*\d+\]/gi, '')
    .replace(LEAKED_BOS_MARKER_PATTERN, '')
    .replace(LEAKED_THOUGHT_MARKER_PATTERN, '')
    .replace(LEAKED_META_MARKER_PATTERN, '');
}

function asString(v) {
  if (typeof v === 'string') {
    return v.trim();
  }
  if (Array.isArray(v)) {
    return asString(v[0]);
  }
  if (v == null) {
    return '';
  }
  return String(v).trim();
}

module.exports = {
  asContentString,
  asString,
  stripReferenceMarkersText,
  stripThinkTags,
};
