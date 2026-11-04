'use strict';

// Cancel without waiting for a stalled source's cancellation promise. Pending
// web-stream reads are settled synchronously by cancel(), before releasing it.
function cancelReader(reader) {
  try { Promise.resolve(reader.cancel()).catch(error => console.warn('[upstream_body] cancel failed', error.message)); }
  catch (error) { console.warn('[upstream_body] cancel failed', error.message); }
}

async function readUpstreamError(response, read = reader => reader.read()) {
  if (!response?.body) return '';
  const reader = response.body.getReader();
  const chunks = [];
  const limit = 64 * 1024;
  let total = 0;
  try {
    while (total < limit) {
      const { done, value } = await read(reader);
      if (done) break;
      if (!value?.length) continue;
      const bytes = value.subarray(0, limit - total);
      chunks.push(bytes);
      total += bytes.length;
    }
    return Buffer.concat(chunks).toString('utf8').trim();
  } finally { cancelReader(reader); reader.releaseLock(); }
}

module.exports = { cancelReader, readUpstreamError };
