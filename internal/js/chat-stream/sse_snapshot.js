'use strict';

function isContentSnapshot(chunk) {
  const op = typeof chunk.o === 'string' ? chunk.o.toUpperCase() : '';
  if (op === 'SET') return true;
  if (op === 'APPEND') return false;
  let value = chunk.v;
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  if (value.response && typeof value.response === 'object') value = value.response;
  return Array.isArray(value.fragments);
}

module.exports = { isContentSnapshot };
