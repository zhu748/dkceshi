'use strict';

const { releaseStreamLease } = require('./http_internal');
const { waitForDrain } = require('./response_flow');

// A usage preview keeps the lease alive until output has actually been written.
async function previewUsage(req, leaseID, meta, fallback, accounting) {
  if (!accounting.enabled) return fallback;
  try {
    const result = await releaseStreamLease(req, leaseID, { ...meta, usage_only: true });
    return accounting.authoritative(fallback, result);
  } catch (error) {
    console.warn('[vercel_stream] usage preview failed', error.message);
    return fallback;
  }
}

async function writeSuccessfulTerminal(res, sendFrame, payload) {
  if (!sendFrame(payload)) throw Object.assign(new Error('Response terminal write failed.'), { code: 'client_write_error' });
  if (res.writableEnded || res.destroyed) throw Object.assign(new Error('Response closed before terminal completion.'), { code: 'client_write_error' });
  try {
    res.write('data: [DONE]\n\n');
    res.flush?.();
    await waitForDrain(res);
  } catch (error) {
    error.code = 'client_write_error';
    throw error;
  }
}

module.exports = { previewUsage, writeSuccessfulTerminal };
