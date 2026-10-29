#!/usr/bin/env node
/* env-snapshot.cjs — 同一份 API 探测序列分别跑在真 Chromium 与 vm 模拟环境，
 * diff 两边的结果，定位指纹字段露馅的源头 API。 */
'use strict';
const fs = require('fs');
const path = require('path');
const vm = require('node:vm');
const zlib = require('node:zlib');
const crypto = require('node:crypto');
const DEVID_DIR = 'data/devid';

const PROBE = `
(function () {
    function s(v) { try { return String(v); } catch (e) { return '<throw:' + (e && e.message) + '>'; } }
    var out = {};
    out.keys_document = s(Object.keys(document));
    out.keys_navigator_len = Object.keys(navigator).length;
    out.keys_window_len = Object.keys(window).length;
    out.in_window_ontouchstart = s('ontouchstart' in window);
    out.typeof_ontouchstart = s(typeof window.ontouchstart);
    out.plugins_len = s(navigator.plugins.length);
    out.plugin_names = s(Array.prototype.map.call(navigator.plugins, function (p) { return p.name; }));
    out.mime_types = s(Array.prototype.map.call(navigator.mimeTypes, function (m) { return m.type; }));
    out.enumerateDevices_toString = s(navigator.mediaDevices && navigator.mediaDevices.enumerateDevices && navigator.mediaDevices.enumerateDevices.toString().slice(0, 90));
    out.getBattery_toString = s(navigator.getBattery && navigator.getBattery.toString().slice(0, 90));
    out.createElement_toString = s(document.createElement.toString().slice(0, 90));
    out.getElementById_toString = s(document.getElementById.toString().slice(0, 90));
    out.intl_tz = s(Intl.DateTimeFormat().resolvedOptions().timeZone);
    out.intl_locale = s(Intl.DateTimeFormat().resolvedOptions().locale);
    out.intl_hourCycle = s(Intl.DateTimeFormat().resolvedOptions().hourCycle);
    out.matchmedia_dh = s(matchMedia('(dynamic-range: high)').matches);
    out.matchmedia_pt = s(matchMedia('(prefers-reduced-transparency: reduce)').matches);
    out.matchmedia_gamut = s(matchMedia('(color-gamut: p3)').matches);
    out.matchmedia_over = s(matchMedia('(overflow-block: scroll)').matches);
    out.matchmedia_pointer = s(matchMedia('(any-pointer: fine)').matches);
    out.matchmedia_update = s(matchMedia('(update: fast)').matches);
    out.matchmedia_str = s(matchMedia.toString().slice(0, 90));
    out.notif_perm = s(typeof Notification !== 'undefined' ? Notification.permission : '<no Notification>');
    out.permissions_query = s((navigator.permissions && navigator.permissions.query) ? navigator.permissions.query.toString().slice(0, 80) : '<none>');
    out.storage_getDir = s(navigator.storage && navigator.storage.getDirectory ? navigator.storage.getDirectory.toString().slice(0, 80) : '<none>');
    out.gpads = s(navigator.getGamepads ? navigator.getGamepads.toString().slice(0, 80) : '<none>');
    out.gpads_call = s(navigator.getGamepads ? JSON.stringify(navigator.getGamepads()) : '<none>');
    out.keys_window_chrome = s(typeof window.chrome === 'object' && window.chrome ? Object.keys(window.chrome) : '<no chrome obj>');
    out.xr_is_session = s(navigator.xr && navigator.xr.isSessionSupported ? 'yes' : '<none>');
    out.connection = s(navigator.connection ? JSON.stringify({ rtt: navigator.connection.rtt, down: navigator.connection.downlink, et: navigator.connection.effectiveType }) : '<none>');
    out.userActivation = s(navigator.userActivation ? JSON.stringify({ hasBeenActive: navigator.userActivation.hasBeenActive, isActive: navigator.userActivation.isActive }) : '<none>');
    out.devicestate = s(navigator.deviceMemory);
    out.getInstalledRelatedApps = s(typeof navigator.getInstalledRelatedApps);
    out.wakeLock = s(typeof navigator.wakeLock);
    out.ink = s(typeof navigator.ink);
    out.hid = s(typeof navigator.hid);
    out.usb = s(typeof navigator.usb);
    out.serial = s(typeof navigator.serial);
    out.bluetooth = s(typeof navigator.bluetooth);
    out.midi = s(typeof navigator.requestMIDIAccess);
    out.credentials = s(typeof navigator.credentials);
    out.clipboard = s(typeof navigator.clipboard);
    out.wtf = s(typeof window.ReportingObserver);
    out.tco = s(typeof window.TrustedCoins);
    out.canShare = s(typeof navigator.canShare);
    out.mediaSession = s(typeof navigator.mediaSession);
    out.presentation = s(typeof navigator.presentation);
    out.protocols_reg = s(typeof navigator.registerProtocolHandler);
    return out;
})()
`;

function toBuffer(ab) { if (Buffer.isBuffer(ab)) return ab; if (ab instanceof ArrayBuffer) return Buffer.from(new Uint8Array(ab)); if (ab && ab.buffer) return Buffer.from(new Uint8Array(ab.buffer, ab.byteOffset || 0, ab.byteLength)); return Buffer.from(ab); }
function toAB(buf) { const o = Buffer.from(buf); return o.buffer.slice(o.byteOffset, o.byteOffset + o.byteLength); }
function log(...a) { process.stderr.write('[env] ' + a.join(' ') + '\n'); }

async function runVm() {
    let sdk = fs.readFileSync(path.join(DEVID_DIR, 'fp-1.min.js'), 'utf8');
    sdk = sdk.split("_0x80aaf5('priId',").join("_0x80aaf5('priId', __leak.priId =");
    const profile = JSON.parse(fs.readFileSync('/tmp/devid-round2.json', 'utf8')).profile;
    process.env.TZ = profile.tzName || 'Asia/Shanghai';
    const ctx = vm.createContext({});
    const prelude = `
        var __profile = ${JSON.stringify(JSON.stringify(profile))};
        var __START = ${Date.now() - 3000};
        var __TZ_OFFSET = ${Number(profile.tzOffset)};
        var __SELFTEST = false;
        var __siteOverride = {"site":"chat.deepseek.com","origin":"https://chat.deepseek.com","pageUrl":"https://chat.deepseek.com/sign_in","pageReferrer":"https://chat.deepseek.com/"};
        var __smAppId='default'; var __smOrg='P9usCUBauxft8eAmUXaZ';
        var __smPubKey=${JSON.stringify(fs.readFileSync(path.join(DEVID_DIR, 'public_key'), 'utf8').trim())};
        var __smProtocol='https'; var __smApiHost='fp-it-acc.portal101.cn'; var __smApiPath='/deviceprofile/v4';
        var __fonts=${JSON.stringify(fs.readFileSync(path.join(DEVID_DIR, 'fonts.json'), 'utf8'))};
        var __gpuPool=${JSON.stringify(fs.readFileSync(path.join(DEVID_DIR, 'gpus.json'), 'utf8'))};
        var __leak={};
    `;
    vm.runInContext(prelude, ctx, { filename: 'p.js' });
    let nid = 1; const timers = new Map();
    const mk = (lf, iv) => (fn, ms, ...a) => { const id = nid++; const h = lf(() => { if (!iv) timers.delete(id); try { if (typeof fn === 'function') fn(...a); } catch (e) {} }, Math.max(0, Number(ms) || 0)); timers.set(id, h); return id; };
    ctx.__setTimeout = mk(setTimeout, false); ctx.__clearTimeout = (id) => { const h = timers.get(id); if (h) { clearTimeout(h); timers.delete(id); } };
    ctx.__setInterval = mk(setInterval, true); ctx.__clearInterval = (id) => { const h = timers.get(id); if (h) { clearInterval(h); timers.delete(id); } };
    ctx.__stackFrames = () => [];
    ctx.__zlibDeflate = (ab) => toAB(zlib.deflateSync(toBuffer(ab)));
    ctx.__randomBytes = (n) => toAB(crypto.randomBytes(Math.max(0, n | 0)));
    vm.runInContext(fs.readFileSync(path.join(DEVID_DIR, 'browser_env.js'), 'utf8'), ctx, { filename: 'browser_env.js' });
    return vm.runInContext(PROBE, ctx, { filename: 'probe.js' });
}

async function runReal() {
    const candidates = ['playwright', 'playwright-core'];
    const roots = ['/usr/local/lib/node_modules', '/usr/lib/node_modules', '/home/z/.npm-global/lib/node_modules'];
    try { roots.push(require('child_process').execSync('npm root -g', { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim()); } catch (_) {}
    let pw = null;
    for (const r of ['', ...roots]) { for (const m of ['playwright', 'playwright-core']) { try { pw = pw || require(path.join(r, m)); } catch (_) {} } }
    if (!pw) throw new Error('no playwright');
    const browser = await pw.chromium.launch({ headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-blink-features=AutomationControlled'] });
    try {
        const ctx2 = await browser.newContext({
            userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
            locale: 'zh-CN', timezoneId: 'Asia/Shanghai',
        });
        const page = await ctx2.newPage();
        try { await page.goto('https://chat.deepseek.com/sign_in', { waitUntil: 'domcontentloaded' }); } catch (e) { log('goto:', e.message.split('\n')[0]); }
        return await page.evaluate(PROBE);
    } finally { await browser.close().catch(() => {}); }
}

(async () => {
    const [real, vmOut] = [await runReal(), await runVm()];
    const keys = [...new Set([...Object.keys(real), ...Object.keys(vmOut)])].sort();
    let diff = 0;
    for (const k of keys) {
        const r = String(real[k]), v = String(vmOut[k]);
        if (r === v) continue;
        diff++;
        console.log('✗ ' + k);
        console.log('   real: ' + r.slice(0, 150));
        console.log('   vm  : ' + v.slice(0, 150));
    }
    console.log('\nAPI 差异: ' + diff + '/' + keys.length);
    fs.writeFileSync('/tmp/env-diff-real.json', JSON.stringify(real, null, 1));
    fs.writeFileSync('/tmp/env-diff-vm.json', JSON.stringify(vmOut, null, 1));
})().catch(e => { console.error('ERR', e); process.exit(1); });
