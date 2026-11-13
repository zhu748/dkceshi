'use strict';

const { VERSION } = require('./devid-android-env.cjs');

// Resolution and GPU chip names: vivo official parameter pages, checked 2026-10-29.
// DPR, browser renderer spelling and font metrics remain simulation presets.
const VIVO_SPECS = require('../internal/config/android_device_specs.json').map(d =>
    [d.model, d.android_version, d.product, d.width, d.height, d.gpu]);

const ANDROID_FONTS = {
    _comment: 'Android font-name compatibility template; no Windows measured metrics. Widths use the simulator approximation.',
    baseFonts: ['Roboto', 'Noto Sans', 'Noto Serif', 'Droid Sans Mono', 'Arial', 'Times New Roman', 'Courier New'],
    eastAsianLocales: ['zh-CN', 'zh-TW', 'ja-JP', 'ko-KR'],
    eastAsianFonts: ['Noto Sans CJK SC', 'Noto Sans CJK TC', 'Noto Sans CJK JP', 'Noto Sans CJK KR'],
    genericFamilies: { monospace: [43.2, 84], 'sans-serif': [48, 84], serif: [52, 84] },
    familyAdvances: { default: 0.52, serif: 0.56, sans: 0.5, mono: 0.6 },
};

function localeSettings(anchor, spec = {}, at = new Date()) {
    const locale = String(anchor.locale || 'zh-CN');
    const locales = spec.locales || [];
    const loc = locales.find((l) => l.code === locale && l.tz === anchor.tz)
        || locales.find((l) => l.code === locale) || {};
    const langs = loc.langs || [...new Set([locale, locale.split('-')[0]])];
    const tzName = String(anchor.tz || loc.tz || 'Asia/Shanghai');
    const formatter = new Intl.DateTimeFormat(locale, { timeZone: tzName, timeZoneName: 'longOffset' });
    const label = formatter.formatToParts(at).find(p => p.type === 'timeZoneName').value;
    const offset = /(?:GMT|UTC)(?:([+-])(\d{2}):(\d{2}))?$/.exec(label);
    // A fixed English locale ensures localized GMT spellings cannot affect parsing.
    const english = offset || /GMT(?:([+-])(\d{2}):(\d{2}))?$/.exec(new Intl.DateTimeFormat('en-US', { timeZone: tzName, timeZoneName: 'longOffset' }).formatToParts(at).find(p => p.type === 'timeZoneName').value);
    if (!english) throw new Error('无法解析时区偏移: ' + tzName);
    const currentOffset = english[1] ? (Number(english[2]) * 60 + Number(english[3])) * (english[1] === '+' ? -1 : 1) : 0;
    const tzOffset = Number(anchor.tz_offset ?? currentOffset);
    if (!Number.isFinite(tzOffset) || Math.abs(tzOffset) > 840) throw new Error('非法 tz_offset');
    return { locale, lang: { code: locale, list: langs.slice() }, langs: langs.slice(),
        acceptLanguage: loc.acceptLanguage || langs.map((l, i) => l + (i ? ';q=' + Math.max(0.1, 1 - i / 10).toFixed(1) : '')).join(','), tzName, tzOffset };
}

const GENERIC_GPU_CHIPS = [
    'Adreno 660', 'Adreno 730', 'Adreno 740', 'Adreno 750',
    'Immortalis-G715', 'Mali-G615',
];

// 池外型号没有官方参数表；从常见移动 GPU 驱动串中按档案种子挑选（同档案稳定），
// 避免所有 generic 机型共用一个在真实 Android Chrome 上几乎不出现的字符串。
// 无随机源时按型号哈希确定性映射：同一型号始终对应同一 GPU。
function genericGpuChip(model, rand) {
    let chip;
    if (typeof rand === 'function') {
        chip = GENERIC_GPU_CHIPS[Math.floor(rand() * GENERIC_GPU_CHIPS.length)];
    } else {
        let h = 0;
        for (let i = 0; i < model.length; i++) h = (h * 31 + model.charCodeAt(i)) >>> 0;
        chip = GENERIC_GPU_CHIPS[h % GENERIC_GPU_CHIPS.length];
    }
    return {
        vendor: chip.startsWith('Adreno') ? 'Qualcomm' : 'ARM',
        renderer: chip.startsWith('Adreno') ? chip.replace('Adreno ', 'Adreno (TM) ') : chip,
    };
}

function mobileHardware(anchor, spec = {}, gpuPool = {}, rand = null) {
    const model = String(anchor.device_model || '').trim();
    const known = VIVO_SPECS.find((d) => d[0] === model);
    const screen = anchor.screen || (spec.modelScreens || {})[model]
        || (known ? [Math.round(known[3] / 3), Math.round(known[4] / 3), 3] : [360, 800, 3]);
    if (!Array.isArray(screen) || screen.length !== 3 || !screen.every((n) => Number.isFinite(n) && n > 0)
        || screen[0] > 4000 || screen[1] > 4000 || screen[2] > 8) throw new Error('非法 Android screen [CSS宽,CSS高,DPR]');
    const chip = known ? known[5] : null;
    const gpu = anchor.gpu || (gpuPool.modelGpus || {})[model] || (known ? {
        vendor: chip.startsWith('Adreno') ? 'Qualcomm' : 'ARM',
        renderer: chip.startsWith('Adreno') ? chip.replace('Adreno ', 'Adreno (TM) ') : chip,
    } : genericGpuChip(model, rand));
    if (!gpu || typeof gpu.vendor !== 'string' || typeof gpu.renderer !== 'string' || !gpu.renderer
        || /Direct3D|D3D11|D3D12/i.test(gpu.renderer)) throw new Error('非法 Android GPU 模板');
    return { screen, gpu: { vendor: gpu.vendor, renderer: gpu.renderer }, hardwareSource: known
        ? 'https://www.vivo.com.cn/vivo/param/' + known[2] : 'generic-mobile-template',
        hardwareApproximate: true };
}

function buildAndroidProfile(anchor, spec, gpuPool, seed, rand) {
    const model = String(anchor.device_model || '').trim();
    const androidVersion = Number(anchor.android_version);
    if (!model || !Number.isInteger(androidVersion) || androidVersion < 7 || androidVersion > 100) {
        throw new Error('Android 锚定必须提供 device_model 和 android_version（Android 主版本，非 API level）');
    }
    const hardware = mobileHardware(anchor, spec, gpuPool, rand);
    const [screenW, screenH, dpr] = hardware.screen;
    const pick = (a) => a[Math.floor(rand() * a.length)];
    const innerH = Math.max(1, screenH - 80);
    return {
        androidAdapterVersion: VERSION, osFamily: 'android', deviceModel: model, androidVersion,
        ...localeSettings(anchor, spec), ...hardware,
        screenW, screenH, dpr, availW: screenW, availH: screenH,
        outerW: screenW, outerH: screenH, innerW: screenW, innerH,
        colorDepth: 24, cores: 8, deviceMemory: 8, maxTouchPoints: 5,
        storageQuotaBytes: 64 * 1024 ** 3, jsHeapSizeLimit: 2 * 1024 ** 3,
        canvasSeed: Math.floor(rand() * 0xffffffff), seed,
        gpu: hardware.gpu, battery: { charging: rand() < 0.5, level: pick([0.55, 0.72, 0.87, 1]) },
        rtt: pick([50, 100, 150]), downlink: pick([4.6, 7.8, 10]), effType: '4g',
    };
}

module.exports = { ANDROID_FONTS, VIVO_SPECS, localeSettings, mobileHardware, buildAndroidProfile };
