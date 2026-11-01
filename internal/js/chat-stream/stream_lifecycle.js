'use strict';

const { createLeaseReleaser } = require('./http_internal');

// Track disconnects before prepare starts. A late prepare result may still
// contain a lease which must be released even when no upstream stream starts.
function createStreamLifecycle(req, res, getMeta) {
  const controller = new AbortController();
  let closed = Boolean(req.aborted || res.destroyed || res.writableEnded);
  let reader = null;
  let releaseLease = async () => {};
  const logCancelError = error => console.warn('[vercel_stream] reader cancel failed', error.message);
  const cancelReader = () => {
    if (!reader || typeof reader.cancel !== 'function') return;
    try { Promise.resolve(reader.cancel()).catch(logCancelError); }
    catch (error) { logCancelError(error); }
  };
  const close = () => {
    if (closed) return;
    closed = true;
    controller.abort();
    cancelReader();
  };
  const onResponseClose = () => { if (!res.writableEnded) close(); };
  req.on('aborted', close);
  res.on('close', onResponseClose);
  res.on('error', close);
  if (closed) controller.abort();

  return {
    get closed() { return closed; },
    get signal() { return controller.signal; },
    get reader() { return reader; },
    set reader(value) { reader = value; },
    trackLease(leaseID) { releaseLease = createLeaseReleaser(req, leaseID, getMeta); },
    release() { return releaseLease(); },
    async dispose() {
      req.removeListener('aborted', close);
      res.removeListener('close', onResponseClose);
      res.removeListener('error', close);
      closed = true;
      controller.abort();
      cancelReader();
      await releaseLease();
    },
  };
}

module.exports = { createStreamLifecycle };
