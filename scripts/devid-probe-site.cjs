#!/usr/bin/env node
/*
 * devid-probe-site.cjs — 一次性站点配置抓取（此后运行时不再需要浏览器）。
 *
 * 用真实 Chromium 打开 chat.deepseek.com/sign_in，抓三样东西：
 *   1. window._smConf（数美 SDK 初始化参数：appId/organization/publicKey/apiHost/apiPath）
 *   2. fp-it-acc.portal101.cn/deviceprofile/v4 上报的真实请求体与响应（协议样本）
 *   3. localStorage 里的 smidV2 等键（档案复用参考）
 *
 * stdout 一行 JSON：{ok, sm_conf, captured, storage}
 */
'use strict';

const fs = require('fs');

function log(...a) { process.stderr.write('[probe] ' + a.join(' ') + '\n'); }

function resolvePlaywright() {
    const candidates = [];
    if (process.env.PLAYWRIGHT_MODULE) candidates.push(process.env.PLAYWRIGHT_MODULE);
    candidates.push('playwright', 'playwright-core');
    const roots = ['/usr/local/lib/node_modules', '/usr/lib/node_modules',
        '/home/z/.npm-global/lib/node_modules', '/home/z/.bun/install/global/node_modules'];
    try {
        const { execSync } = require('child_process');
        const out = execSync('npm root -g', { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
        if (out) roots.push(out);
    } catch (_) {}
    for (const r of roots) for (const m of ['playwright', 'playwright-core']) candidates.push(require('path').join(r, m));
    for (const c of candidates) { try { return require(c); } catch (_) {} }
    return null;
}

async function main() {
    const pw = resolvePlaywright();
    if (!pw) { finish(false, 'playwright not found'); return; }

    const browser = await pw.chromium.launch({
        headless: true,
        args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-blink-features=AutomationControlled'],
    });
    const context = await browser.newContext({
        userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
        viewport: { width: 1366, height: 768 },
        locale: 'zh-CN',
        timezoneId: 'Asia/Shanghai',
    });
    await context.addInitScript(() => {
        Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
        window.chrome = window.chrome || { runtime: {} };
    });
    const page = await context.newPage();
    page.setDefaultTimeout(60000);

    const captured = [];
    page.on('response', async (resp) => {
        try {
            const url = resp.url();
            if (url.indexOf('deviceprofile') < 0) return;
            const req = resp.request();
            captured.push({
                url,
                status: resp.status(),
                method: req.method(),
                request_headers: await req.allHeaders(),
                request_body: req.postData() || '',
                response_body: (await resp.text()).slice(0, 4000),
            });
            log('捕获上报:', url, '→', resp.status());
        } catch (e) { log('resp 抓取失败:', e.message); }
    });

    log('打开 sign_in …');
    try { await page.goto('https://chat.deepseek.com/sign_in', { waitUntil: 'domcontentloaded' }); }
    catch (e) { log('goto:', e.message.split('\n')[0]); }

    log('等待 SMSdk …');
    try {
        await page.waitForFunction('typeof SMSdk !== "undefined" && SMSdk.getDeviceId', null, { timeout: 45000, polling: 500 });
    } catch (e) { log('SMSdk 等待超时:', e.message.split('\n')[0]); }

    // 给上报响应留点时间
    await page.waitForTimeout(5000);

    let smConf = null, smExtra = null, storage = {};
    try {
        smConf = await page.evaluate('JSON.parse(JSON.stringify(window._smConf || null))');
        smExtra = await page.evaluate(`(function(){
            var out = {};
            try { out.sdk_keys = Object.keys(window.SMSdk || {}); } catch(e) {}
            try { out.device_id = window.SMSdk.getDeviceId ? window.SMSdk.getDeviceId() : null; } catch(e) {}
            return out;
        })()`);
        storage = await page.evaluate('JSON.parse(JSON.stringify(window.localStorage || {}))');
    } catch (e) { log('evaluate:', e.message); }

    await browser.close().catch(() => {});
    finish(true, '', { sm_conf: smConf, sdk: smExtra, captured, storage });
}

function finish(ok, err, data) {
    process.stdout.write(JSON.stringify(ok ? { ok: true, ...data } : { ok: false, error: err }) + '\n');
    process.exit(ok ? 0 : 1);
}
main().catch((e) => finish(false, e && e.message ? e.message : String(e)));
