'use strict';

const { spawn } = require('node:child_process');
const path = require('node:path');
const readline = require('node:readline');

// One child per warm Node worker, shared by its concurrent invocations. Control
// calls never traverse Vercel's load balancer and cannot lose their lease owner.
function createLocalControl({ launch = spawn, timeoutMs = 20000, binary = path.join(__dirname, '../../runtime/vercel-control') } = {}) {
  let pending = null;
  let child = null;
  let generation = 0;
  const start = () => {
    if (pending) return pending;
    const current = ++generation;
    const startup = new Promise((resolve, reject) => {
      const worker = launch(binary, [], { stdio: ['pipe', 'pipe', 'inherit'], env: { ...process.env, VERCEL: '1' } });
      child = worker;
      const lines = readline.createInterface({ input: worker.stdout });
      let ready = false;
      let settled = false;
      const fail = (error) => {
        if (current === generation) { pending = null; child = null; }
        if (!settled) { settled = true; reject(error); }
        clearTimeout(timer);
        lines.close();
      };
      const timer = setTimeout(() => {
        fail(new Error('local Go control startup timed out'));
        worker.kill();
      }, timeoutMs);
      worker.once('error', fail);
      worker.once('exit', (code, signal) => fail(new Error(`local Go control exited (${signal || code})`)));
      worker.stdin.on('error', error => { fail(error); worker.kill(); });
      lines.on('line', (line) => {
        if (ready) return;
        try {
          const { origin } = JSON.parse(line);
          const url = new URL(origin);
          if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || !url.port || url.username || url.password || url.pathname !== '/') return;
          ready = true;
          settled = true;
          clearTimeout(timer);
          resolve(url.origin);
        } catch { /* Only the ready JSON line is part of the startup protocol. */ }
      });
    });
    // Also handle synchronous launch failures without retaining a rejected cache.
    pending = startup;
    startup.catch(() => { if (current === generation) pending = null; });
    return startup;
  };
  const close = () => {
    generation++;
    pending = null;
    const running = child;
    child = null;
    if (running) { running.stdin.end(); running.kill(); }
  };
  return { start, close };
}

const control = createLocalControl();
module.exports = { localControlOrigin: () => control.start(), createLocalControl };
