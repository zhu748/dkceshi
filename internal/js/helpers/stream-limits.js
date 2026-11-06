'use strict';

const defaults = require('../../streambudget/defaults.json');

function streamLimit(key) {
  const raw = process.env['DS2API_STREAM_' + key.toUpperCase()];
  const value = Number(raw);
  return /^\d+$/.test(raw || '') && Number.isSafeInteger(value) && value > 0 && value <= 2 ** 30 ? value : defaults[key];
}

module.exports = { streamLimit };
