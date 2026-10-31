'use strict';

const crypto = require('node:crypto');
const VERSION = 1;
const MARKER = '/* ds2api Android Web SDK adapter v1 */';
const LEGACY_HASH = '7ccb74480bfeb26a7ad9188c6842fa6b88684db55da20cf9d78bf4f7c5231645';

// Timestamp values are Numbers, not necessarily fractional. Adding a random
// fraction to an epoch timestamp can round to an integer at double precision.
// Validate the API contract instead of randomly rejecting a valid clock.
function normalizeClockProbes(source) {
    return source
        .replace("['timeOrigin 是浮点', 'performance.timeOrigin % 1 !== 0', true]",
            "['timeOrigin 是有限非负时间戳', 'Number.isFinite(performance.timeOrigin) && performance.timeOrigin >= 0', true]")
        .replace("['performance.now 亚毫秒', 'performance.now() % 1 !== 0', true]",
            "['performance.now 是有限非负计时', 'Number.isFinite(performance.now()) && performance.now() >= 0', true]");
}

// Patch only the supplied, known environment. An unknown version must be reviewed.
// The SDK itself is unchanged; these are mobile browser compatibility templates.
function adaptBrowserEnv(source) {
    let s = source.replace(/\r\n/g, '\n');
    if (s.startsWith(MARKER)) return normalizeClockProbes(s);
    if (crypto.createHash('sha256').update(s).digest('hex') !== LEGACY_HASH) {
        // Newer external environments are checked through their observable API.
        if (s.includes('__siteOverride') && /android\s*:/.test(s)) return s;
        throw new Error('未知 browser_env.js 版本，拒绝套用旧版 Android 补丁');
    }
    const replace = (from, to) => {
        const count = s.split(from).length - 1;
        if (count !== 1) throw new Error('Android 环境补丁命中数异常: ' + from.slice(0, 50) + ' (' + count + ')');
        s = s.replace(from, () => to);
    };
    replace('var ID = (function () {', `OS_FAMILIES.android = {
        os: 'android', platform: 'Linux armv8l', uaPlatform: 'Android', chPlatform: '"Android"',
        ua: function (v) { return 'Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/' + v + '.0.0.0 Mobile Safari/537.36'; }
    };
    var ID = (function () {`);
    replace('var langs = (profile.langs && profile.langs.length) ? profile.langs : loc.langs;',
        'var langs = (profile.langs && profile.langs.length) ? profile.langs : ((profile.lang && profile.lang.list) || loc.langs);');
    replace('function __requestHeaders() {', `if (typeof __siteOverride === 'object' && __siteOverride) {
        ['site', 'origin', 'pageUrl', 'pageReferrer'].forEach(function (key) {
            if (__siteOverride[key]) CONF[key] = __siteOverride[key];
        });
    }
    function __requestHeaders() {`);
    replace("'sec-ch-ua-mobile': '?0'", "'sec-ch-ua-mobile': ID.os === 'android' ? '?1' : '?0'");
    replace('var __GPU = (function () {', `var __GPU = (function () {
        if (ID.os === 'android') {
            if (!profile.gpu || !profile.gpu.renderer) throw new Error('Android 档案缺少 GPU 模板');
            return profile.gpu;
        }`);
    replace('var PLUGIN_DEFS = [', "var PLUGIN_DEFS = ID.os === 'android' ? [] : [");
    replace("    arr[0] = makeMimeView('application/pdf', 'pdf', 'Portable Document Format');", `    if (ID.os === 'android') {
        Object.defineProperty(arr, 'length', { value: 0, configurable: true });
        Object.defineProperty(arr, Symbol.toStringTag, { value: 'MimeTypeArray', configurable: true });
        return arr;
    }
    arr[0] = makeMimeView('application/pdf', 'pdf', 'Portable Document Format');`);
    replace("    Object.defineProperty(proto, Symbol.toStringTag, { value: 'PluginArray', configurable: true });", `    if (ID.os === 'android') Object.keys(proto).forEach(function (key) {
        if (typeof proto[key] !== 'function') delete proto[key];
    });
    Object.defineProperty(proto, Symbol.toStringTag, { value: 'PluginArray', configurable: true });`);
    replace("pathname: '/sign_up'", "pathname: CONF.pageUrl.replace(CONF.origin, '').split(/[?#]/)[0] || '/'");
    replace('maxTouchPoints: 0,', "maxTouchPoints: ID.os === 'android' ? profile.maxTouchPoints : 0,");
    replace('pdfViewerEnabled: true,', "pdfViewerEnabled: ID.os !== 'android',");
    // Both the low-entropy object and getHighEntropyValues must agree.
    s = s.replaceAll('mobile: false,', "mobile: ID.os === 'android',");
    replace("architecture: 'x86', bitness: '64', formFactors: ['Desktop'],", "architecture: ID.os === 'android' ? 'arm' : 'x86', bitness: '64', formFactors: [ID.os === 'android' ? 'Mobile' : 'Desktop'],");
    replace("model: '', platformVersion: '10.0.0',", "model: ID.os === 'android' ? profile.deviceModel : '', platformVersion: ID.os === 'android' ? profile.androidVersion + '.0.0' : '10.0.0',");
    replace('fullVersionList: ID.brands.slice(),', "fullVersionList: ID.brands.map(function (b) { return { brand: b.brand, version: b.version + '.0.0.0' }; }),");
    replace('var __STORAGE_QUOTA = (120 +', 'var __STORAGE_QUOTA = profile.storageQuotaBytes || (120 +');
    replace('jsHeapSizeLimit: 4294705152,', 'jsHeapSizeLimit: profile.jsHeapSizeLimit || 4294705152,');
    replace("        if (/prefers-color-scheme:", `        if (ID.os === 'android' && /(?:any-)?pointer:\\s*(coarse|fine|none)/.test(query)) matches = /pointer:\\s*coarse/.test(query);
        else if (ID.os === 'android' && /(?:any-)?hover:\\s*(hover|none)/.test(query)) matches = /hover:\\s*none/.test(query);
        else if (/prefers-color-scheme:`);
    replace('delete windowMock.AudioContext;', `if (ID.os === 'android') {
        windowMock.ontouchstart = null;
        Object.defineProperty(documentInterface.proto, 'ontouchstart', { get: function () { return null; }, configurable: true, enumerable: true });
        windowMock.TouchEvent = nativeFn(function TouchEvent(type) { this.type = String(type || ''); }, 'TouchEvent');
        windowMock.Touch = nativeFn(function Touch(init) { if (init) Object.assign(this, init); }, 'Touch');
    }
    delete windowMock.AudioContext;`);
    replace("'navigator.mimeTypes[0].enabledPlugin === navigator.plugins[0]'", "'ID.os === \"android\" ? navigator.plugins.length === 0 && navigator.mimeTypes.length === 0 : navigator.mimeTypes[0].enabledPlugin === navigator.plugins[0]'");
    replace("'navigator.plugins.length', 5", "'navigator.plugins.length', ID.os === 'android' ? 0 : 5");
    replace("'navigator.mimeTypes.length', 2", "'navigator.mimeTypes.length', ID.os === 'android' ? 0 : 2");
    replace('var ua = navigator.userAgent; if (ua.indexOf("Windows NT")', 'var ua = navigator.userAgent; if (ua.indexOf("Android") >= 0) return navigator.platform === "Linux armv8l"; if (ua.indexOf("Windows NT")');
    replace('var ua = navigator.userAgent, p = navigator.userAgentData.platform; if (ua.indexOf("Windows NT")', 'var ua = navigator.userAgent, p = navigator.userAgentData.platform; if (ua.indexOf("Android") >= 0) return p === "Android"; if (ua.indexOf("Windows NT")');
    replace('.getParameter(0x9246).indexOf("ANGLE") >= 0', '.getParameter(0x9246).length > 0');
    return normalizeClockProbes(MARKER + '\n' + s);
}

module.exports = { adaptBrowserEnv, VERSION };
