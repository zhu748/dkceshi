'use strict';

const http2 = require('node:http2');
const { createHash } = require('node:crypto');

// Exclusive checkout keeps aborting one request from resetting an active
// sibling. Idle entries retain only hashed identity keys, never raw tokens.
function createAppSessionPool({ maxIdle = 64, perIdentity = 2, idleMs = 90000 } = {}) {
  const idle = new Map(), entries = new Set();
  const remove = entry => {
    clearTimeout(entry.timer);
    entries.delete(entry);
    const list = idle.get(entry.key);
    if (list) {
      const index = list.indexOf(entry);
      if (index >= 0) list.splice(index, 1);
      if (!list.length) idle.delete(entry.key);
    }
  };
  return {
    take(origin, headers, installation = '') {
      const identity = [origin, installation, ...['authorization', 'x-device-id', 'x-rangers-id', 'x-device-model', 'user-agent'].map(name => headers[name] || '')];
      const key = createHash('sha256').update(JSON.stringify(identity)).digest('hex');
      let entry;
      while (idle.get(key)?.length) {
        const candidate = idle.get(key).at(-1);
        remove(candidate);
        if (!candidate.session.closed && !candidate.session.destroyed && !candidate.retired) { entry = candidate; break; }
        candidate.session.destroy();
      }
      const reused = Boolean(entry);
      if (!entry) {
        entry = { key, session: http2.connect(origin), retired: false };
        entry.session.on('error', () => { entry.retired = true; remove(entry); entry.session.destroy(); });
        entry.session.on('close', () => remove(entry));
        entry.session.on('goaway', () => {
          entry.retired = true;
          if (entries.has(entry)) { remove(entry); entry.session.destroy(); }
        });
      }
      entry.session.ref();
      let released = false;
      return {
        session: entry.session, reused,
        release(reusable) {
          if (released) return;
          released = true;
          if (!reusable || entry.retired || entry.session.closed || entry.session.destroyed || entries.size >= maxIdle || (idle.get(key)?.length || 0) >= perIdentity) {
            entry.session.destroy();
            return;
          }
          if (!idle.has(key)) idle.set(key, []);
          idle.get(key).push(entry);
          entries.add(entry);
          entry.timer = setTimeout(() => { remove(entry); entry.session.destroy(); }, idleMs);
          entry.timer.unref();
          entry.session.unref();
        },
      };
    },
    closeIdle() { for (const entry of [...entries]) { remove(entry); entry.session.destroy(); } },
    get size() { return entries.size; },
  };
}

module.exports = { createAppSessionPool };
