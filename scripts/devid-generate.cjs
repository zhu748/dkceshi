#!/usr/bin/env node
/* Android Web SDK runner: offline VM collection, account-routed capture, and
 * standalone reporting. Run --selftest for an offline payload check or
 * --check-assets for the runtime readiness probe. */
'use strict';

const fs = require('fs');
const path = require('path');
const vm = require('node:vm');
const crypto = require('node:crypto');
// DS2API_DEVID_CAPTURE_V1: account-routed SDK reporting is supported.
const { parseArgs } = require('./devid-cli.cjs');
const { loadArchive, selectAnchoredArchive, saveArchive } = require('./devid-state.cjs');
const { makeHostPrims } = require('./devid-host.cjs');
const { runViaBrowser, sendViaChromium } = require('./devid-browser.cjs');
const { checkAssets } = require('./devid-assets.cjs');
const { checkRuntime } = require('./devid-runtime-check.cjs');
const { adaptBrowserEnv, VERSION } = require('./devid-android-env.cjs');
const { ANDROID_FONTS } = require('./devid-android-profile.cjs');

const DEVID_DIR = path.resolve(String(process.env.DS2API_DEVID_DIR || '').trim() || path.join(__dirname, '..', 'data', 'devid'));
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
/* ---------------- 档案（设备身份持久化） ---------------- */
/* ---------------- 宿主原语 ---------------- */
/* ============================================================
 * 主流程
 * ============================================================ */
if (require.main === module) {
    Promise.resolve().then(() => {
        const opts = parseArgs(process.argv.slice(2));
        return opts.browser ? runViaBrowser(opts, log, finish) : mainVm(opts);
    }).catch(e => finish(false, e.message || String(e)));
}

module.exports = { selectAnchoredArchive, makeHostPrims };

function readData(name) { return fs.readFileSync(path.join(DEVID_DIR, name), 'utf8'); }

async function mainVm(o) {
    const assetCheck = checkAssets(DEVID_DIR, o);
    if (o.checkAssets) {
        finish(true, '', undefined, { assets_verified: assetCheck.verified, node_version: process.versions.node });
        return;
    }
    const deadline = Date.now() + o.timeout * 1000;

    /* --- 档案与画像 --- */
    const arch = loadArchive(o, log);
    const profilesSpec = JSON.parse(readData('profiles.json'));
    const gpuPool = JSON.parse(readData('gpus.json'));
    let anchor = null;
    if (o.anchor) {
        try { anchor = JSON.parse(fs.readFileSync(o.anchor, 'utf8')); }
        catch (e) { throw new Error('anchor 文件解析失败: ' + e.message); }
        if (!anchor || typeof anchor !== 'object' || Array.isArray(anchor)) throw new Error('anchor 必须是设备配置对象');
    }
    const selected = selectAnchoredArchive(arch, anchor, profilesSpec, gpuPool, crypto.randomBytes(4).readUInt32BE(0));
    const profile = selected.profile;
    if (selected.smidV2) profile.smidV2 = selected.smidV2;
    log('画像:', profile.osFamily, profile.deviceModel || '', profile.screenW + 'x' + profile.screenH, 'dpr=' + profile.dpr,
        'cores=' + profile.cores, profile.locale, profile.smidV2 ? '(复用 smid)' : '(新设备)',
        profile.gpu ? 'gpu=' + String(profile.gpu.renderer).slice(6, 40) : '');
    if (profile.hardwareSource === 'generic-mobile-template') log('注意: 此型号无硬件参数表，使用通用移动模拟模板，需真机校准');
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
    const prims = makeHostPrims(log);
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
        var __fonts = ${JSON.stringify(JSON.stringify(ANDROID_FONTS))};
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
    vm.runInContext(adaptBrowserEnv(readData('browser_env.js')), ctx, { filename: 'browser_env.js', timeout: 5000 });
    const runtimeChecks = await checkRuntime(ctx, profile, SM_CONF.siteOverride);

    /* --- selftest：以检测方视角体检环境，然后退出 --- */
    if (o.selftest) {
        const results = vm.runInContext('JSON.parse(JSON.stringify(__probeResults))', ctx);
        results.push(...runtimeChecks.map((r) => ({ ...r, expr: 'runtime identity', expect: true, actual: r.ok })));
        let bad = 0;
        for (const r of results) {
            log((r.ok ? '✓ ' : '✗ ') + r.name + '  [' + r.expr + '] expect=' + r.expect + ' actual=' + String(r.actual).slice(0, 60));
            if (!r.ok) bad++;
        }
        log('environment checks: ' + (results.length - bad) + '/' + results.length + ' 通过');
        o.selftestResult = { selftest: true, pass: results.length - bad, total: results.length };
        if (bad) { finish(false, 'runtime identity selftest failed'); return; }
    }
    const incompatible = runtimeChecks.filter((r) => !r.ok);
    if (incompatible.length) {
        throw new Error('SDK 扩展包与设备/站点配置不兼容: ' + incompatible.map((r) => r.name).join('、') + '；请提供支持 Android 锚定和 __siteOverride 的扩展包');
    }

    /* --- verify.js（debug 体检用，非 debug 也装载：不影响 SDK） --- */
    if (o.debug) vm.runInContext(readData('verify.js'), ctx, { filename: 'verify.js' });

    log('运行数美 SDK…');
    vm.runInContext(sdkSrc, ctx, { filename: 'fp-1.min.js', timeout: Math.max(1, Math.min(5000, deadline - Date.now())) });

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
    if (captured.url !== 'https://' + SM_CONF.apiHost + SM_CONF.apiPath || String(captured.body).length > 2 * 1024 * 1024) throw new Error('SDK 上报地址或载荷无效');
    JSON.parse(captured.body);
    if (o.selftest) {
        finish(true, '', undefined, { ...o.selftestResult, sdk_payload: true, assets_verified: assetCheck.verified, node_version: process.versions.node });
        return;
    }
    if (o.capture) {
        const smidV2 = String(vm.runInContext('(lsMap && lsMap.smidV2) || (cookieJar && cookieJar.smidV2) || ""', ctx) || '');
        if (o.profile) saveArchive(o.profile, { version: 3, profile, smid_v2: smidV2, updated_at: new Date().toISOString() });
        const headers = vm.runInContext('JSON.parse(JSON.stringify(__requestHeaders()))', ctx);
        finish(true, '', undefined, { report: { url: captured.url, body: String(captured.body), headers }, mode: 'capture', environment: 'android-web-sdk' });
        return;
    }


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

    if (o.dumpPayload) {
        fs.mkdirSync(path.dirname(o.dumpPayload), { recursive: true });
        fs.writeFileSync(o.dumpPayload, JSON.stringify({ captured, fingerprint: verifyOut && verifyOut.fingerprint,
            environment: 'android-web-sdk', runtime_checks: runtimeChecks }, null, 2), { mode: 0o600 });
    }

    /* --- 真实转发数美云端：两种通道 ---
     * 默认：Node fetch（快，但 TLS/HTTP2 指纹是 Node 的）
     * --via-chromium：用真 Chromium 页面内的 fetch 发（真浏览器 TLS/H2，慢，实验用） */
    const reqHeaders = vm.runInContext('JSON.parse(JSON.stringify(__requestHeaders()))', ctx);
    let respText = '', respStatus = 0;
    if (o.viaChromium) {
        log('经真 Chromium 页面转发（TLS 对照实验）…');
        const via = await sendViaChromium(captured, reqHeaders, o, log);
        respStatus = via.status; respText = via.body;
    } else {
        log('上报数美:', SM_CONF.apiHost, '…');
        const resp = await fetch(captured.url, {
            method: 'POST',
            headers: reqHeaders,
            body: captured.body,
            signal: AbortSignal.timeout(Math.max(1, deadline - Date.now())),
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
    if (o.profile) saveArchive(o.profile, { version: 3, profile, smid_v2: smidV2, updated_at: new Date().toISOString() });

    finish(true, '', deviceId, {
        smid_v2: smidV2,
        endpoint: captured.url,
        verify_problems: verifyOut ? verifyOut.problems || [] : null,
        detail_c: detailCode,
        mode: 'vm',
        environment: 'android-web-sdk',
        hardware_source: profile.hardwareSource,
        hardware_approximate: true,
    });
}

function sleep(ms) { return new Promise((r) => setTimeout(r, ms)); }

function finish(ok, err, deviceId, extra) {
    const out = ok
        ? { ok: true, device_id: deviceId, elapsed_ms: Date.now() - FINISH_T0, ...(extra || {}) }
        : { ok: false, error: err, elapsed_ms: Date.now() - FINISH_T0 };
    process.stdout.write(JSON.stringify(out) + '\n', () => process.exit(ok ? 0 : 1));
}

/* ============================================================
 * Playwright 兜底路径（--browser；环境 100% 真实，起浏览器慢）
 * ============================================================ */
