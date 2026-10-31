'use strict';

// Canonical and generic XML element extraction.
const { toRawString } = require('./state');
const {
  skipXmlIgnoredSection,
} = require('./cdata_scan');
const {
  findXmlTagEnd,
  hasXmlTagBoundary,
  isSelfClosingXmlTag,
} = require('./markup_syntax');

function findXmlElementBlocks(text, tag) {
  const source = toRawString(text);
  const name = toRawString(tag).trim().toLowerCase();
  if (!source || !name) {
    return [];
  }
  const out = [];
  let pos = 0;
  while (pos < source.length) {
    const start = findXmlStartTagOutsideCDATA(source, name, pos);
    if (!start) {
      break;
    }
    const end = findMatchingXmlEndTagOutsideCDATA(source, name, start.bodyStart);
    if (!end) {
      pos = start.bodyStart;
      continue;
    }
    out.push({
      attrs: start.attrs,
      body: source.slice(start.bodyStart, end.closeStart),
      start: start.start,
      end: end.closeEnd,
    });
    pos = end.closeEnd;
  }
  return out;
}

function findXmlStartTagOutsideCDATA(text, tag, from) {
  const lower = text.toLowerCase();
  const target = `<${tag}`;
  for (let i = Math.max(0, from || 0); i < text.length;) {
    const skipped = skipXmlIgnoredSection(text, i);
    if (skipped.blocked) {
      return null;
    }
    if (skipped.advanced) {
      i = skipped.next;
      continue;
    }
    if (lower.startsWith(target, i) && hasXmlTagBoundary(text, i + target.length)) {
      const tagEnd = findXmlTagEnd(text, i + target.length);
      if (tagEnd < 0) {
        return null;
      }
      return {
        start: i,
        bodyStart: tagEnd + 1,
        attrs: text.slice(i + target.length, tagEnd),
      };
    }
    i += 1;
  }
  return null;
}

function findMatchingXmlEndTagOutsideCDATA(text, tag, from) {
  const lower = text.toLowerCase();
  const openTarget = `<${tag}`;
  const closeTarget = `</${tag}`;
  let depth = 1;
  for (let i = Math.max(0, from || 0); i < text.length;) {
    const skipped = skipXmlIgnoredSection(text, i);
    if (skipped.blocked) {
      return null;
    }
    if (skipped.advanced) {
      i = skipped.next;
      continue;
    }
    if (lower.startsWith(closeTarget, i) && hasXmlTagBoundary(text, i + closeTarget.length)) {
      const tagEnd = findXmlTagEnd(text, i + closeTarget.length);
      if (tagEnd < 0) {
        return null;
      }
      depth -= 1;
      if (depth === 0) {
        return { closeStart: i, closeEnd: tagEnd + 1 };
      }
      i = tagEnd + 1;
      continue;
    }
    if (lower.startsWith(openTarget, i) && hasXmlTagBoundary(text, i + openTarget.length)) {
      const tagEnd = findXmlTagEnd(text, i + openTarget.length);
      if (tagEnd < 0) {
        return null;
      }
      if (!isSelfClosingXmlTag(text.slice(i, tagEnd))) {
        depth += 1;
      }
      i = tagEnd + 1;
      continue;
    }
    i += 1;
  }
  return null;
}

function findGenericXmlElementBlocks(text) {
  const source = toRawString(text);
  if (!source) {
    return [];
  }
  const out = [];
  let pos = 0;
  while (pos < source.length) {
    const start = findGenericXmlStartTagOutsideCDATA(source, pos);
    if (!start) {
      break;
    }
    if (start.selfClosing) {
      out.push({
        name: start.name,
        localName: start.localName,
        attrs: start.attrs,
        body: '',
        start: start.start,
        end: start.end + 1,
      });
      pos = start.end + 1;
      continue;
    }
    const end = findMatchingGenericXmlEndTagOutsideCDATA(source, start.name, start.bodyStart);
    if (!end) {
      pos = start.bodyStart;
      continue;
    }
    out.push({
      name: start.name,
      localName: start.localName,
      attrs: start.attrs,
      body: source.slice(start.bodyStart, end.closeStart),
      start: start.start,
      end: end.closeEnd,
    });
    pos = end.closeEnd;
  }
  return out;
}

function findGenericXmlStartTagOutsideCDATA(text, from) {
  const lower = text.toLowerCase();
  for (let i = Math.max(0, from || 0); i < text.length;) {
    const skipped = skipXmlIgnoredSection(text, i);
    if (skipped.blocked) {
      return null;
    }
    if (skipped.advanced) {
      i = skipped.next;
      continue;
    }
    if (text[i] !== '<' || text[i + 1] === '/' || text[i + 1] === '!' || text[i + 1] === '?') {
      i += 1;
      continue;
    }
    const match = text.slice(i + 1).match(/^([A-Za-z_][A-Za-z0-9_.:-]*)/);
    if (!match) {
      i += 1;
      continue;
    }
    const name = match[1];
    const nameEnd = i + 1 + name.length;
    if (!hasXmlTagBoundary(text, nameEnd)) {
      i += 1;
      continue;
    }
    const tagEnd = findXmlTagEnd(text, nameEnd);
    if (tagEnd < 0) {
      return null;
    }
    return {
      start: i,
      end: tagEnd,
      bodyStart: tagEnd + 1,
      name,
      localName: name.includes(':') ? name.slice(name.lastIndexOf(':') + 1) : name,
      attrs: text.slice(nameEnd, tagEnd),
      selfClosing: isSelfClosingXmlTag(text.slice(i, tagEnd)),
    };
  }
  return null;
}

function findMatchingGenericXmlEndTagOutsideCDATA(text, name, from) {
  const lower = text.toLowerCase();
  const needle = toRawString(name).trim().toLowerCase();
  if (!needle) {
    return null;
  }
  const openTarget = `<${needle}`;
  const closeTarget = `</${needle}`;
  let depth = 1;
  for (let i = Math.max(0, from || 0); i < text.length;) {
    const skipped = skipXmlIgnoredSection(text, i);
    if (skipped.blocked) {
      return null;
    }
    if (skipped.advanced) {
      i = skipped.next;
      continue;
    }
    if (lower.startsWith(closeTarget, i) && hasXmlTagBoundary(text, i + closeTarget.length)) {
      const tagEnd = findXmlTagEnd(text, i + closeTarget.length);
      if (tagEnd < 0) {
        return null;
      }
      depth -= 1;
      if (depth === 0) {
        return { closeStart: i, closeEnd: tagEnd + 1 };
      }
      i = tagEnd + 1;
      continue;
    }
    if (lower.startsWith(openTarget, i) && hasXmlTagBoundary(text, i + openTarget.length)) {
      const tagEnd = findXmlTagEnd(text, i + openTarget.length);
      if (tagEnd < 0) {
        return null;
      }
      if (!isSelfClosingXmlTag(text.slice(i, tagEnd))) {
        depth += 1;
      }
      i = tagEnd + 1;
      continue;
    }
    i += 1;
  }
  return null;
}

module.exports = {
  findGenericXmlElementBlocks,
  findXmlElementBlocks,
};
