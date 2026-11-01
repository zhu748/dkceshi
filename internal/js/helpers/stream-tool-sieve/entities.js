'use strict';

const names = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };
const windows1252 = [0x20ac,0x81,0x201a,0x192,0x201e,0x2026,0x2020,0x2021,0x2c6,0x2030,0x160,0x2039,0x152,0x8d,0x17d,0x8f,0x90,0x2018,0x2019,0x201c,0x201d,0x2022,0x2013,0x2014,0x2dc,0x2122,0x161,0x203a,0x153,0x9d,0x17e,0x178];

// Decode one XML entity layer. Replacement output is never decoded again.
function unescapeHtml(text) {
  return text.replace(/&(amp|lt|gt|quot|apos|#(?:[0-9]+|[xX][0-9a-fA-F]+));/g, (_match, entity) => {
    if (names[entity]) return names[entity];
    const hex = entity[1]?.toLowerCase() === 'x';
    let code = Number.parseInt(entity.slice(hex ? 2 : 1), hex ? 16 : 10);
    if (!code || code > 0x10ffff || (code >= 0xd800 && code <= 0xdfff)) return '\ufffd';
    if (code >= 0x80 && code <= 0x9f) code = windows1252[code - 0x80];
    return String.fromCodePoint(code);
  });
}

module.exports = { unescapeHtml };
