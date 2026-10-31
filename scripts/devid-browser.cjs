'use strict';
const fs = require('node:fs');
const path = require('node:path');
const { saveArchive } = require('./devid-state.cjs');

/* 经真 Chromium 页面 fetch 发上报：隔离 TLS/HTTP2 指纹变量（实验通道）。
 * 页面 fetch 的 sec-ch-ua 与 UA 由浏览器自身决定（与旧 Playwright 方案同等程度的不一致）。
 */
async function sendViaChromium(captured, reqHeaders, o, log) {
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

async function runViaBrowser(o, log, finish) {
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
        if (o.profile) saveArchive(o.profile, await context.storageState());
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

module.exports = { runViaBrowser, sendViaChromium };
