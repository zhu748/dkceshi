'use strict';

const vm = require('node:vm');

// Check what the SDK can actually observe, rather than trusting injected globals.
async function checkRuntime(ctx, profile, site) {
    const snapshot = await vm.runInContext(`(async function () {
        var hints = navigator.userAgentData;
        var high = hints && hints.getHighEntropyValues
            ? await hints.getHighEntropyValues(['model', 'platformVersion']) : {};
        var gl = document.createElement('canvas').getContext('webgl');
        return JSON.stringify({
            ua: navigator.userAgent, platform: navigator.platform,
            language: navigator.language, offset: new Date().getTimezoneOffset(),
            mobile: hints && hints.mobile, hintPlatform: hints && hints.platform,
            model: high.model, platformVersion: high.platformVersion,
            origin: location.origin, url: location.href, referrer: document.referrer,
            headers: __requestHeaders(),
            renderer: gl && gl.getParameter(0x9246)
        });
    })()`, ctx, { filename: 'devid-runtime-check.js', timeout: 5000 });
    return checkSnapshot(JSON.parse(snapshot), profile, site);
}

function checkSnapshot(actual, profile, site) {
    const checks = [];
    const add = (name, ok) => checks.push({ name, ok: Boolean(ok) });
    const headers = Object.fromEntries(Object.entries(actual.headers || {}).map(([k, v]) => [k.toLowerCase(), v]));
    add('SDK 站点 origin', actual.origin === site.origin);
    add('SDK 站点 URL', actual.url === site.pageUrl);
    add('SDK 站点 referrer', actual.referrer === site.pageReferrer);
    add('上报 Origin', headers.origin === site.origin);
    add('上报 Referer', headers.referer === site.pageReferrer);
    add('上报 UA 与浏览器 UA 一致', headers['user-agent'] === actual.ua);
    add('浏览器语言与档案一致', actual.language === profile.locale);
    add('浏览器时区偏移与档案一致', actual.offset === Number(profile.tzOffset));
    if (profile.osFamily === 'android') {
        add('Android 移动 UA', /Android/.test(actual.ua) && /Mobile/.test(actual.ua));
        add('Android navigator.platform', /^(Linux arm|Android)/i.test(actual.platform));
        add('Android Client Hints 平台', actual.hintPlatform === 'Android');
        add('Android Client Hints mobile', actual.mobile === true);
        add('Android 上报平台', headers['sec-ch-ua-platform'] === '"Android"');
        add('Android 上报 mobile', headers['sec-ch-ua-mobile'] === '?1');
        add('Android 高熵型号与档案一致', actual.model === profile.deviceModel);
        add('Android 高熵版本与档案一致', Number(String(actual.platformVersion).split('.')[0]) === Number(profile.androidVersion));
        add('Android GPU 未退回 Direct3D', typeof actual.renderer === 'string' && !/Direct3D|D3D11|D3D12/i.test(actual.renderer));
        if (profile.gpu && profile.gpu.renderer) add('Android GPU 与档案一致', actual.renderer === profile.gpu.renderer);
    }
    return checks;
}

module.exports = { checkRuntime, checkSnapshot };
