#!/usr/bin/env node
/*
 * devid-probe-real.cjs — 真实环境指纹明文提取（diff 基准）。
 *
 * Playwright 真 Chromium 打开 chat.deepseek.com/sign_in，
 * route 拦截 fp.min.js → 注入带 priId 泄露补丁的本地同版本 SDK，
 * 真实环境采集上报后：
 *   - 抓上报 body + __leak.priId + 响应 deviceId
 * 输出 /tmp/real-fp.json：{body, pri_id, device_id}
 * 之后离线解密与 vm 版指纹逐字段 diff。
 */
'use strict';
const fs = require('fs');

function log(...a) { process.stderr.write('[probe-real] ' + a.join(' ') + '\n'); }

function resolvePlaywright() {
    const candidates = ['playwright', 'playwright-core'];
    const roots = ['/usr/local/lib/node_modules', '/usr/lib/node_modules', '/home/z/.npm-global/lib/node_modules'];
    try {
        const out = require('child_process').execSync('npm root -g', { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
        if (out) roots.push(out);
    } catch (_) {}
    for (const r of roots) for (const m of ['playwright', 'playwright-core']) candidates.push(require('path').join(r, m));
    for (const c of candidates) { try { return require(c); } catch (_) {} }
    return null;
}

(async () => {
    const pw = resolvePlaywright();
    if (!pw) throw new Error('playwright not found');

    /* 补丁版 SDK（与线上同 md5，仅泄露 priId） */
    let sdk = fs.readFileSync('data/devid/fp-1.min.js', 'utf8');
    const find = "_0x80aaf5('priId',";
    if (sdk.indexOf(find) < 0) throw new Error('SDK 补丁定位串失效（版本变化）');
    sdk = sdk.split(find).join("_0x80aaf5('priId', __leak.priId =");
    sdk = 'window.__leak = {};\n' + sdk;

    const browser = await pw.chromium.launch({
        headless: true,
        args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-blink-features=AutomationControlled'],
    });
    const out = {};
    try {
        const context = await browser.newContext({
            userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
            viewport: { width: 1366, height: 768 }, locale: 'zh-CN', timezoneId: 'Asia/Shanghai',
        });
        const page = await context.newPage();
        page.setDefaultTimeout(60000);

        let captured = false;
        page.on('response', async (resp) => {
            try {
                if (captured || resp.url().indexOf('deviceprofile') < 0) return;
                const j = JSON.parse(await resp.text());
                out.device_id = j && j.detail && j.detail.deviceId || '';
                out.resp_status = resp.status();
                const req = resp.request();
                out.body = req.postData() || '';
                captured = true;
                log('捕获真实上报:', resp.status(), 'deviceId:', out.device_id ? out.device_id.slice(0, 20) + '…' : '(无)');
            } catch (e) { log('resp 抓取失败:', e.message); }
        });

        /* 拦截 fp.min.js → 补丁版 */
        await page.route('**/fp*.js', async (route) => {
            log('拦截 SDK 加载:', route.request().url());
            await route.fulfill({ status: 200, contentType: 'application/javascript; charset=utf-8', body: sdk });
        });

        log('打开 sign_in …');
        try { await page.goto('https://chat.deepseek.com/sign_in', { waitUntil: 'domcontentloaded' }); }
        catch (e) { log('goto:', e.message.split('\n')[0]); }

        const t0 = Date.now();
        while (!out.body && Date.now() - t0 < 45000) await new Promise(r => setTimeout(r, 500));
        if (!out.body) throw new Error('未捕获到上报');

        /* 读泄露的密钥 */
        out.pri_id = await page.evaluate('window.__leak && window.__leak.priId ? window.__leak.priId : ""');
        log('priId:', out.pri_id ? out.pri_id : '(未泄露!)');

        /* 真实环境的关键对照值（便于诊断） */
        out.env = await page.evaluate(`(function () {
            var c = document.createElement('canvas').getContext('2d');
            return {
                ua: navigator.userAgent, platform: navigator.platform,
                cores: navigator.hardwareConcurrency, memory: navigator.deviceMemory,
                langs: navigator.languages.join(','),
                dpr: window.devicePixelRatio, inner: window.innerWidth + 'x' + window.innerHeight,
                screen: window.screen.width + 'x' + window.screen.height + '(avail ' + window.screen.availWidth + 'x' + window.screen.availHeight + ')',
                tz: Intl.DateTimeFormat().resolvedOptions().timeZone,
                webdriver: navigator.webdriver,
                touch: navigator.maxTouchPoints,
            };
        })()`);

        fs.writeFileSync('/tmp/real-fp.json', JSON.stringify(out));
        console.log(JSON.stringify({ ok: true, device_id: out.device_id || '', pri_id: out.pri_id || '', body_len: out.body.length, env: out.env }));
    } finally { await browser.close().catch(() => {}); }
})().catch(e => { console.log(JSON.stringify({ ok: false, error: e.message })); process.exit(1); });
