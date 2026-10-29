'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { checkSnapshot } = require('../../scripts/devid-runtime-check.cjs');

const site = { origin: 'https://chat.deepseek.com', pageUrl: 'https://chat.deepseek.com/sign_in', pageReferrer: 'https://chat.deepseek.com/' };
const profile = { osFamily: 'android', deviceModel: 'V2405A', androidVersion: 15, locale: 'zh-CN', tzOffset: -480 };
function mobile() {
    return {
        ua: 'Mozilla/5.0 (Linux; Android 15; V2405A) Chrome/153.0.0.0 Mobile Safari/537.36',
        platform: 'Linux armv8l', hintPlatform: 'Android', mobile: true,
        model: 'V2405A', platformVersion: '15.0.0', renderer: 'Adreno (TM) 750',
        language: 'zh-CN', offset: -480,
        origin: site.origin, url: site.pageUrl, referrer: site.pageReferrer,
        headers: { Origin: site.origin, Referer: site.pageReferrer, 'User-Agent': 'Mozilla/5.0 (Linux; Android 15; V2405A) Chrome/153.0.0.0 Mobile Safari/537.36', 'sec-ch-ua-platform': '"Android"', 'sec-ch-ua-mobile': '?1' },
    };
}

test('accepts a runtime whose observable identity matches its Android anchor', () => {
    assert.deepEqual(checkSnapshot(mobile(), profile, site).filter((r) => !r.ok), []);
});

test('rejects legacy Windows runtime even when an Android profile was injected', () => {
    const actual = { ...mobile(), ua: 'Mozilla/5.0 (Windows NT 10.0) Chrome/153.0.0.0', platform: 'Win32', hintPlatform: 'Windows', mobile: false, model: '', platformVersion: '10.0.0', renderer: 'ANGLE (NVIDIA, Direct3D11)' };
    actual.headers = { ...actual.headers, 'User-Agent': actual.ua, 'sec-ch-ua-platform': '"Windows"', 'sec-ch-ua-mobile': '?0' };
    const failures = checkSnapshot(actual, profile, site).filter((r) => !r.ok);
    assert.equal(failures.length, 9);
    assert.ok(failures.some((r) => r.name.includes('型号')));
});

test('rejects ignored site overrides, stale model/version, and mismatching headers', () => {
    const actual = { ...mobile(), origin: 'https://platform.deepseek.com', url: 'https://platform.deepseek.com/sign_up', model: 'V2507A', platformVersion: '14.0.0' };
    actual.headers = { ...actual.headers, Origin: actual.origin, 'User-Agent': 'other' };
    const names = checkSnapshot(actual, profile, site).filter((r) => !r.ok).map((r) => r.name);
    assert.equal(names.length, 6);
    assert.ok(names.includes('上报 UA 与浏览器 UA 一致'));
});
