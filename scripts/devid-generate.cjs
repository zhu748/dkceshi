#!/usr/bin/env node
/*
 * devid-generate.cjs — 无浏览器 device_id 生成器（轻量版）。
 *
 * 运行链：Node vm + browser_env.js(环境模拟) + fp-1.min.js(数美 SDK)
 *   → SDK 读 window._smConf 自初始化，采集"指纹"并加密打包
 *   → XHR 被劫持进 __captured（不发网络）
 *   → 本脚本以真实 HTTPS 把载荷转发到数美云端 fp-it-acc.portal101.cn/deviceprofile/v4
 *   → 响应 code=1100 的 detail.deviceId 即 device_id
 *   → 数美/本地(verify.js) 双重体检通过才算成功
 *
 * 相对 Playwright 方案：无浏览器进程，秒级、~60MB 内存；指纹由高保真模拟
 * 环境生成（fonts/gpus 真机池 + V8 栈伪装 + canvas PNG 抖动 + 站点身份注入）。
 *
 * 用法：
 *   node devid-generate.cjs [--reset] [--profile <file>] [--timeout <秒>]
 *                           [--debug] [--selftest] [--browser]
 *   --browser    旧 Playwright 路径（应急兜底；生成慢但环境 100% 真实）
 *
 * 档案文件（--profile）JSON：
 *   { "version": 3, "profile": {<browser_env 画像>}, "smid_v2": "..." }
 * 兼容读旧 Playwright storageState（从中提取 localStorage.smidV2）。
 *
 * stdout 输出一行 JSON：{"ok":true,"device_id":"...","smid_v2":"...",...}
 * 过程日志全部走 stderr。
 */
'use strict';

const fs = require('fs');
const path = require('path');
const vm = require('node:vm');
const zlib = require('node:zlib');
const crypto = require('node:crypto');

const DEVID_DIR = path.join(__dirname, '..', 'data', 'devid');
const FINISH_T0 = Date.now();

/* 数美站点配置（2026-10 探针自 chat.deepseek.com 实抓，与扩展包 public_key 一致） */
const SM_CONF = {
    appId: 'default',
    organization: 'P9usCUBauxft8eAmUXaZ',
    apiHost: 'fp-it-acc.portal101.cn',
    apiPath: '/deviceprofile/v4',
    protocol: 'https',
    publicKeyFile: path.join(DEVID_DIR, 'public_key'),
    /* 上报身份：对齐 chat.deepseek.com（要登录的站点） */
    siteOverride: {
        site: 'chat.deepseek.com',
        origin: 'https://chat.deepseek.com',
        pageUrl: 'https://chat.deepseek.com/sign_in',
        pageReferrer: 'https://chat.deepseek.com/',
    },
};

function log(...args) { process.stderr.write('[devid] ' + args.join(' ') + '\n'); }

/* ---------------- 参数 ---------------- */
function parseArgs(argv) {
    const opts = { reset: false, profile: '', timeout: 120, debug: false, selftest: false, browser: false };
    for (let i = 0; i < argv.length; i++) {
        const a = argv[i];
        if (a === '--reset') opts.reset = true;
        else if (a === '--profile') opts.profile = String(argv[++i] || '');
        else if (a === '--timeout') opts.timeout = Number(argv[++i]) || 120;
        else if (a === '--debug') opts.debug = true;
        else if (a === '--selftest') opts.selftest = true;
        else if (a === '--browser') opts.browser = true;
        else if (a === '--via-chromium') opts.viaChromium = true;
        else if (a === '--dump-payload') { const f = String(argv[++i] || ''); if (f) { opts.dumpPayload = f; } }
        else if (a === '--help' || a === '-h') {
            process.stdout.write('usage: node devid-generate.cjs [--reset] [--profile <file>] [--timeout <sec>] [--debug] [--selftest] [--browser]\n');
            process.exit(0);
        }
    }
    return opts;
}

/* ---------------- 档案（设备身份持久化） ---------------- */
function loadArchive(opts) {
    /* 返回 {profile, smidV2} 或全新档案 */
    if (!opts.profile || opts.reset) {
        if (opts.profile && opts.reset && fs.existsSync(opts.profile)) {
            try { fs.unlinkSync(opts.profile); log('已删除旧档案（重置设备）:', opts.profile); } catch (_) {}
        }
        return { profile: null, smidV2: '' };
    }
    if (!fs.existsSync(opts.profile)) return { profile: null, smidV2: '' };
    try {
        const raw = JSON.parse(fs.readFileSync(opts.profile, 'utf8'));
        /* 新格式 v3 */
        if (raw && typeof raw === 'object' && raw.profile && raw.version === 3) {
            log('复用设备档案(v3):', opts.profile);
            return { profile: raw.profile, smidV2: String(raw.smid_v2 || '') };
        }
        /* 旧 Playwright storageState：提取 localStorage.smidV2（同设备延续） */
        if (raw && Array.isArray(raw.origins)) {
            let smidV2 = '';
            for (const o of raw.origins) {
                if (!Array.isArray(o.localStorage)) continue;
                for (const kv of o.localStorage) if (kv.name === 'smidV2') smidV2 = String(kv.value || '');
            }
            log('旧 storageState 档案：提取 smidV2=', smidV2 ? smidV2.slice(0, 24) + '…' : '(空)');
            return { profile: null, smidV2 };
        }
        log('档案格式不认识，按新设备处理');
        return { profile: null, smidV2: '' };
    } catch (e) {
        log('档案解析失败，按新设备处理:', e.message);
        return { profile: null, smidV2: '' };
    }
}

/* ---------------- 画像挑选（profiles.json 权重表） ---------------- */
function mulberry32(seed) {
    let a = seed >>> 0;
    return function () {
        a |= 0; a = (a + 0x6D2B79F5) | 0;
        let t = Math.imul(a ^ (a >>> 15), 1 | a);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}
function pickWeighted(rand, items, weightKey) {
    let total = 0;
    for (const it of items) total += Number(it[weightKey] || 1);
    let r = rand() * total;
    for (const it of items) { r -= Number(it[weightKey] || 1); if (r <= 0) return it; }
    return items[items.length - 1];
}
function pickFrom(rand, arr) { return arr[Math.floor(rand() * arr.length) % arr.length]; }

function buildProfile(profilesSpec, seed) {
    const rand = mulberry32(seed);
    const fam = pickWeighted(rand, profilesSpec.osFamilies, 'weight');
    const loc = pickWeighted(rand, profilesSpec.locales, 'weight');
    const screen = pickFrom(rand, fam.screens);
    const dpr = pickFrom(rand, fam.dprs);
    const cores = pickFrom(rand, fam.cores);
    const taskbar = pickFrom(rand, fam.taskbar);
    const rtt = pickFrom(rand, profilesSpec.network.rtt);
    const downlink = pickFrom(rand, profilesSpec.network.downlink);
    const charging = rand() < profilesSpec.battery.chargingProbability;
    const level = pickFrom(rand, profilesSpec.battery.levels);

    const screenW = screen[0], screenH = screen[1];
    const availW = screenW, availH = screenH - taskbar;
    const outerW = availW, outerH = availH;
    /* 最大化窗口：screen.width/innerWidth 同为 CSS 像素且页面无滚动条时相等；
     * 高度方向 innerH = outerH - 浏览器 chrome(标签栏+地址栏) */
    const innerW = outerW;
    const innerH = outerH - (80 + Math.floor(rand() * 10));
    return {
        osFamily: fam.key,
        locale: loc.code,
        lang: { code: loc.code, list: loc.langs },
        acceptLanguage: loc.acceptLanguage,
        tzName: loc.tz, tzOffset: loc.tzOffset,
        screenW, screenH, availW, availH,
        outerW, outerH, innerW, innerH,
        dpr, colorDepth: profilesSpec.defaults.colorDepth,
        cores, deviceMemory: cores <= 4 ? 4 : 8,
        canvasSeed: Math.floor(rand() * 0xffffffff),
        seed: Math.floor(rand() * 0xffffffff),
        battery: { charging, level },
        rtt, downlink, effType: profilesSpec.network.effectiveType,
    };
}

/* ---------------- 宿主原语 ---------------- */
function toBuffer(ab) {
    if (Buffer.isBuffer(ab)) return ab;
    if (ab instanceof ArrayBuffer) return Buffer.from(new Uint8Array(ab));
    if (ab && ab.buffer) return Buffer.from(new Uint8Array(ab.buffer, ab.byteOffset || 0, ab.byteLength));
    return Buffer.from(ab);
}
function toAB(buf) {
    const out = Buffer.from(buf);
    return out.buffer.slice(out.byteOffset, out.byteOffset + out.byteLength);
}
function makeHostPrims() {
    /* 定时器：返回宿主句柄，回调在 vm 侧闭包执行（跨 realm 函数调用安全） */
    let nextId = 1;
    const timers = new Map();
    const mk = (loopFn, isInterval) => (fn, ms, ...a) => {
        const id = nextId++;
        const h = loopFn(() => {
            if (!isInterval) timers.delete(id);
            try { if (typeof fn === 'function') fn(...a); }
            catch (e) { log('timer 回调异常:', e && e.message); }
        }, Math.max(0, Number(ms) || 0));
        timers.set(id, h);
        return id;
    };
    const prims = {
        setTimeout: mk(setTimeout, false),
        clearTimeout: (id) => { const h = timers.get(id); if (h) { clearTimeout(h); timers.delete(id); } },
        setInterval: mk(setInterval, true),
        clearInterval: (id) => { const h = timers.get(id); if (h) { clearInterval(h); timers.delete(id); } },
        /* 栈帧：宿主 Error.stack 同步携带 vm 侧帧（文件名=eval 时指定的 filename），
         * 解析为 [[name, pos], …]；browser_env 会渲染成 V8 格式并跳过 frame 0。 */
        stackFrames: () => {
            const lines = (new Error().stack || '').split('\n');
            const out = [];
            for (let i = 1; i < lines.length; i++) {
                const m = lines[i].match(/^\s*at (.+?) \((.+?)\)$/) || lines[i].match(/^\s*at (.+)$/);
                if (!m) continue;
                const name = m[1], pos = m[2] || '';
                if (/node:internal|node:vm|devid-generate|Script\.runIn|Module\.runIn/.test(name + ' ' + pos)) continue;
                out.push([name, pos]);
            }
            return out;
        },
        zlibDeflate: (ab) => toAB(zlib.deflateSync(toBuffer(ab))),
        randomBytes: (n) => toAB(crypto.randomBytes(Math.max(0, n | 0))),
        /* verify.js 的解密原语（--debug 体检用） */
        aesCbcDecryptHex: (hex, key, iv) => {
            try {
                const d = crypto.createDecipheriv('aes-128-cbc',
                    Buffer.from(String(key).slice(0, 16), 'latin1'),
                    Buffer.from(String(iv), 'latin1'));
                d.setAutoPadding(false);
                return toAB(Buffer.concat([d.update(Buffer.from(String(hex), 'hex')), d.final()]));
            } catch (_) { return null; }
        },
        inflate: (ab) => { try { return toAB(zlib.gunzipSync(toBuffer(ab))); } catch (_) { return null; } },
        b64Decode: (s) => { try { return toAB(Buffer.from(String(s), 'base64')); } catch (_) { return null; } },
    };
    return prims;
}

/* ============================================================
 * 主流程
 * ============================================================ */
const opts = parseArgs(process.argv.slice(2));
if (opts.browser) { runViaBrowser(opts).catch((e) => finish(false, e.message || String(e))); }
else { mainVm(opts).catch((e) => finish(false, e.message || String(e))); }

function readData(name) { return fs.readFileSync(path.join(DEVID_DIR, name), 'utf8'); }

async function mainVm(o) {
    const deadline = Date.now() + o.timeout * 1000;

    /* --- 档案与画像 --- */
    const arch = loadArchive(o);
    const profilesSpec = JSON.parse(readData('profiles.json'));
    let profile = arch.profile || buildProfile(profilesSpec, crypto.randomBytes(4).readUInt32BE(0));
    if (arch.smidV2) profile.smidV2 = arch.smidV2;
    log('画像:', profile.osFamily, profile.screenW + 'x' + profile.screenH, 'dpr=' + profile.dpr,
        'cores=' + profile.cores, profile.locale, profile.smidV2 ? '(复用 smid)' : '(新设备)');
    /* Intl 用宿主 ICU：运行时 TZ 决定 resolvedOptions().timeZone，与画像时区保持同源。
     * （Node 13.2+ 支持运行时改 env.TZ，V8/ICU 随之跟随） */
    if (profile.tzName) process.env.TZ = profile.tzName;

    /* --- SDK 补丁（priId 泄露，verify 体检依赖） --- */
    let sdkSrc = readData('fp-1.min.js');
    let patchesSpec = null;
    try { patchesSpec = JSON.parse(readData('patches.json')); } catch (e) { log('patches.json 解析失败:', e.message); }
    if (patchesSpec) {
        for (const p of (patchesSpec.patches || [])) {
            if (p.debug && !o.debug) continue;
            if (sdkSrc.indexOf(p.find) < 0) { log('补丁未命中:', p.find.slice(0, 40), '(换 SDK 版本需重定位)'); continue; }
            sdkSrc = sdkSrc.split(p.find).join(p.replace); /* split/join 避免 $ 序列误解析 */
            log('补丁命中:', (p.reason || p.find.slice(0, 30)));
        }
    }

    /* --- vm 装配 --- */
    const prims = makeHostPrims();
    const ctx = vm.createContext({});
    const prelude = `
        var __profile = ${JSON.stringify(JSON.stringify(profile))};
        var __START = ${Date.now() - 2500 - Math.floor(Math.random() * 2000)};
        var __TZ_OFFSET = ${Number(profile.tzOffset)};
        var __SELFTEST = ${o.selftest ? 'true' : 'false'};
        var __siteOverride = ${JSON.stringify(SM_CONF.siteOverride)};
        var __smAppId = ${JSON.stringify(SM_CONF.appId)};
        var __smOrg = ${JSON.stringify(SM_CONF.organization)};
        var __smPubKey = ${JSON.stringify(fs.readFileSync(SM_CONF.publicKeyFile, 'utf8').trim())};
        var __smProtocol = ${JSON.stringify(SM_CONF.protocol)};
        var __smApiHost = ${JSON.stringify(SM_CONF.apiHost)};
        var __smApiPath = ${JSON.stringify(SM_CONF.apiPath)};
        var __fonts = ${JSON.stringify(readData('fonts.json'))};
        var __gpuPool = ${JSON.stringify(readData('gpus.json'))};
        var __leak = {};
    `;
    vm.runInContext(prelude, ctx, { filename: 'devid-prelude.js' });
    ctx.__setTimeout = prims.setTimeout; ctx.__clearTimeout = prims.clearTimeout;
    ctx.__setInterval = prims.setInterval; ctx.__clearInterval = prims.clearInterval;
    ctx.__stackFrames = prims.stackFrames;
    ctx.__zlibDeflate = prims.zlibDeflate;
    ctx.__randomBytes = prims.randomBytes;
    /* verify.js 解密原语（本地指纹体检） */
    ctx.__aesCbcDecryptHex = prims.aesCbcDecryptHex;
    ctx.__inflate = prims.inflate;
    ctx.__b64Decode = prims.b64Decode;
    /* 原语 toString 伪装（宿主函数源码不能泄给 SDK） */
    vm.runInContext(`
        (function () {
            var wrap = function (fn, name) {
                var native = 'function ' + name + '() { [native code] }';
                try { fn.toString = function () { return native; }; } catch (e) {}
                return fn;
            };
            __setTimeout = wrap(__setTimeout, 'setTimeout');
            __clearTimeout = wrap(__clearTimeout, 'clearTimeout');
            __setInterval = wrap(__setInterval, 'setInterval');
            __clearInterval = wrap(__clearInterval, 'clearInterval');
        })();
    `, ctx, { filename: 'devid-prelude.js' });

    log('装载环境模拟…');
    vm.runInContext(readData('browser_env.js'), ctx, { filename: 'browser_env.js' });

    /* --- selftest：以检测方视角体检环境，然后退出 --- */
    if (o.selftest) {
        const results = vm.runInContext('JSON.parse(JSON.stringify(__probeResults))', ctx);
        let bad = 0;
        for (const r of results) {
            log((r.ok ? '✓ ' : '✗ ') + r.name + '  [' + r.expr + '] expect=' + r.expect + ' actual=' + String(r.actual).slice(0, 60));
            if (!r.ok) bad++;
        }
        log('selftest: ' + (results.length - bad) + '/' + results.length + ' 通过');
        process.stdout.write(JSON.stringify({ ok: bad === 0, selftest: true, pass: results.length - bad, total: results.length }) + '\n');
        process.exit(bad === 0 ? 0 : 1);
    }

    /* --- verify.js（debug 体检用，非 debug 也装载：不影响 SDK） --- */
    if (o.debug) vm.runInContext(readData('verify.js'), ctx, { filename: 'verify.js' });

    log('运行数美 SDK…');
    vm.runInContext(sdkSrc, ctx, { filename: 'fp-1.min.js' });

    /* --- 等上报载荷 --- */
    log('等待指纹采集与上报…');
    let captured = null;
    while (Date.now() < deadline) {
        const n = vm.runInContext('__captured.length', ctx);
        if (Number(n) > 0) {
            captured = vm.runInContext('JSON.parse(JSON.stringify(__captured[0]))', ctx);
            break;
        }
        await sleep(200);
    }
    if (!captured || !captured.body) {
        throw new Error('SDK 未产出上报载荷（超时 ' + o.timeout + 's）');
    }
    log('捕获上报:', captured.url, '载荷', String(captured.body).length, '字节');

    /* --- 本地指纹体检（verify.js + 补丁泄露的密钥） --- */
    let verifyOut = null;
    if (o.debug) {
        try {
            const bodyJson = JSON.parse(captured.body);
            verifyOut = vm.runInContext(`__verify(${JSON.stringify(String(bodyJson.data || ''))}, __leak)`, ctx);
            for (const n of (verifyOut && verifyOut.notes) || []) log('check:', n);
            for (const p of (verifyOut && verifyOut.problems) || []) log('PROBLEM:', p);
            if (verifyOut && verifyOut.fingerprint) log('指纹明文字段数:', Object.keys(verifyOut.fingerprint).length);
        } catch (e) { log('verify 异常（不影响转发）:', e.message); }
    }

    /* --- 真实转发数美云端：两种通道 ---
     * 默认：Node fetch（快，但 TLS/HTTP2 指纹是 Node 的）
     * --via-chromium：用真 Chromium 页面内的 fetch 发（真浏览器 TLS/H2，慢，实验用） */
    const reqHeaders = vm.runInContext('JSON.parse(JSON.stringify(__requestHeaders()))', ctx);
    let respText = '', respStatus = 0;
    if (o.viaChromium) {
        log('经真 Chromium 页面转发（TLS 对照实验）…');
        const via = await sendViaChromium(captured, reqHeaders, o);
        respStatus = via.status; respText = via.body;
    } else {
        log('上报数美:', SM_CONF.apiHost, '…');
        const resp = await fetch(captured.url, {
            method: 'POST',
            headers: reqHeaders,
            body: captured.body,
        });
        respStatus = resp.status; respText = await resp.text();
    }
    log('数美响应:', respStatus, respText.slice(0, 200));
    let deviceId = '', detailCode = -1;
    try {
        const j = JSON.parse(respText);
        if (j && j.code === 1100 && j.detail && j.detail.deviceId) { deviceId = String(j.detail.deviceId); detailCode = j.detail.c; }
        else throw new Error('code=' + (j && j.code) + ' detail=' + JSON.stringify(j && j.detail).slice(0, 200));
    } catch (e) {
        throw new Error('数美响应异常: ' + e.message);
    }
    if (!deviceId) throw new Error('响应中无 deviceId');
    /* 关键格式：SDK 的 getDeviceId() 返回 "B" + detail.deviceId（2026-10 探针实证）。
     * 登录接口校验的是带 B 前缀的完整形式，漏掉会被 RISK_DEVICE_DETECTED 拒绝。 */
    if (!deviceId.startsWith('B')) deviceId = 'B' + deviceId;

    /* --- smidV2 读回（设备身份持久化） --- */
    const smidV2 = String(vm.runInContext('(typeof lsMap !== "undefined" && lsMap.smidV2) ? lsMap.smidV2 : (cookieJar && cookieJar.smidV2) || ""', ctx) || '');
    log('device_id:', deviceId.slice(0, 24) + '…', ' smidV2:', smidV2 ? smidV2.slice(0, 24) + '…' : '(空)', ' detail.c:', detailCode);

    /* --- 档案写回 --- */
    if (o.profile) {
        try {
            fs.mkdirSync(path.dirname(o.profile), { recursive: true });
            fs.writeFileSync(o.profile, JSON.stringify({ version: 3, profile, smid_v2: smidV2, updated_at: new Date().toISOString() }, null, 2));
            log('档案已写回:', o.profile);
        } catch (e) { log('档案写回失败:', e.message); }
    }

    finish(true, '', deviceId, {
        smid_v2: smidV2,
        endpoint: captured.url,
        verify_problems: verifyOut ? verifyOut.problems || [] : null,
        detail_c: detailCode,
        mode: 'vm',
    });
}

function sleep(ms) { return new Promise((r) => setTimeout(r, ms)); }

/* 经真 Chromium 页面 fetch 发上报：隔离 TLS/HTTP2 指纹变量（实验通道）。
 * 页面 fetch 的 sec-ch-ua 与 UA 由浏览器自身决定（与旧 Playwright 方案同等程度的不一致）。
 */
async function sendViaChromium(captured, reqHeaders, o) {
    const pw = resolvePlaywright();
    if (!pw) throw new Error('--via-chromium 需要 playwright');
    const browser = await pw.chromium.launch({ headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-blink-features=AutomationControlled'] });
    try {
        const context = await browser.newContext({
            userAgent: reqHeaders['User-Agent'] || reqHeaders['user-agent'],
            locale: 'zh-CN', timezoneId: 'Asia/Shanghai',
        });
        const page = await context.newPage();
        page.setDefaultTimeout(60000);
        try { await page.goto('https://chat.deepseek.com/sign_in', { waitUntil: 'domcontentloaded' }); } catch (e) { log('goto:', e.message.split('\n')[0]); }
        const result = await page.evaluate(async ({ url, body }) => {
            try {
                const r = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json;charset=UTF-8' }, body });
                return { status: r.status, body: await r.text() };
            } catch (e) { return { status: 0, body: String(e && e.message) }; }
        }, { url: captured.url, body: captured.body });
        return result;
    } finally { await browser.close().catch(() => {}); }
}

function finish(ok, err, deviceId, extra) {
    const out = ok
        ? { ok: true, device_id: deviceId, elapsed_ms: Date.now() - FINISH_T0, ...(extra || {}) }
        : { ok: false, error: err, elapsed_ms: Date.now() - FINISH_T0 };
    process.stdout.write(JSON.stringify(out) + '\n');
    process.exit(ok ? 0 : 1);
}

/* ============================================================
 * Playwright 兜底路径（--browser；环境 100% 真实，起浏览器慢）
 * ============================================================ */
async function runViaBrowser(o) {
    log('走 Playwright 兜底路径…');
    const pw = resolvePlaywright();
    if (!pw) throw new Error('playwright 未安装（--browser 兜底不可用）');
    const browser = await pw.chromium.launch({ headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-blink-features=AutomationControlled'] });
    try {
        const context = await browser.newContext({
            userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
            viewport: { width: 1366, height: 768 }, locale: 'zh-CN', timezoneId: 'Asia/Shanghai',
            extraHTTPHeaders: { 'Accept-Language': 'zh-CN,zh;q=0.9,en;q=0.8' },
        });
        await context.addInitScript(() => { Object.defineProperty(navigator, 'webdriver', { get: () => undefined }); window.chrome = window.chrome || { runtime: {} }; });
        const page = await context.newPage();
        page.setDefaultTimeout(Math.max(10000, o.timeout * 1000));
        try { await page.goto('https://chat.deepseek.com/sign_in', { waitUntil: 'domcontentloaded' }); } catch (e) { log('goto:', e.message.split('\n')[0]); }
        await page.waitForFunction('typeof SMSdk !== "undefined"', null, { timeout: Math.max(5000, o.timeout * 1000), polling: 1000 });
        await page.waitForFunction(() => { if (typeof SMSdk === 'undefined' || typeof SMSdk.getDeviceId !== 'function') return false; const id = SMSdk.getDeviceId(); return typeof id === 'string' && id.trim().length > 0; }, null, { timeout: Math.max(5000, o.timeout * 1000), polling: 1000 });
        const deviceId = String(await page.evaluate('SMSdk.getDeviceId()')).trim();
        if (!deviceId) throw new Error('SMSdk.getDeviceId() 为空');
        if (o.profile) {
            try {
                const state = await context.storageState();
                fs.mkdirSync(path.dirname(o.profile), { recursive: true });
                fs.writeFileSync(o.profile, JSON.stringify(state));
            } catch (e) { log('画像保存失败:', e.message); }
        }
        await browser.close().catch(() => {});
        finish(true, '', deviceId, { mode: 'browser' });
    } catch (e) {
        await browser.close().catch(() => {});
        throw e;
    }
}

function resolvePlaywright() {
    const candidates = [];
    if (process.env.PLAYWRIGHT_MODULE) candidates.push(process.env.PLAYWRIGHT_MODULE);
    candidates.push('playwright', 'playwright-core');
    const roots = ['/usr/local/lib/node_modules', '/usr/lib/node_modules', '/home/z/.npm-global/lib/node_modules', '/home/z/.bun/install/global/node_modules'];
    try {
        const out = require('child_process').execSync('npm root -g', { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
        if (out) roots.push(out);
    } catch (_) {}
    for (const r of roots) for (const m of ['playwright', 'playwright-core']) candidates.push(path.join(r, m));
    for (const c of candidates) { try { return require(c); } catch (_) {} }
    return null;
}
