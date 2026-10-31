'use strict';
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { VERSION } = require('./devid-android-env.cjs');
const { VIVO_SPECS, localeSettings, buildAndroidProfile } = require('./devid-android-profile.cjs');

function loadArchive(opts, log = () => {}) {
    /* 返回 {profile, smidV2} 或全新档案 */
    if (!opts.profile || opts.reset) {
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
        throw new Error('设备档案读取失败: ' + e.message);
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
function buildAnchoredProfile(anchor, profilesSpec, gpuPool, seed) {
    return buildAndroidProfile(anchor, profilesSpec, gpuPool, seed, mulberry32(seed));
}

function selectAnchoredArchive(arch, anchor, profilesSpec, gpuPool, seed) {
    if (!anchor) {
        if (arch.profile && arch.profile.osFamily === 'android' && arch.profile.androidAdapterVersion === VERSION) {
            const saved = arch.profile;
            anchor = { device_model: saved.deviceModel, android_version: saved.androidVersion, locale: saved.locale, tz: saved.tzName };
        }
        const device = VIVO_SPECS[seed % VIVO_SPECS.length];
        if (!anchor) anchor = { device_model: device[0], android_version: device[1], locale: 'zh-CN', tz: 'Asia/Shanghai', tz_offset: -480 };
    }
    const old = arch.profile;
    const sameDevice = old && old.androidAdapterVersion === VERSION && old.osFamily === 'android' && old.deviceModel === anchor.device_model && old.androidVersion === Number(anchor.android_version);
    if (sameDevice) {
        const profile = { ...old, ...localeSettings({ locale: old.locale, tz: old.tzName, ...anchor }, profilesSpec) };
        let smidV2 = arch.smidV2;
        if (anchor.screen || anchor.gpu) {
            const next = buildAnchoredProfile(anchor, profilesSpec, gpuPool, old.seed ?? seed);
            const keys = ['screenW', 'screenH', 'dpr', 'availW', 'availH', 'outerW', 'outerH', 'innerW', 'innerH', 'gpu', 'hardwareSource', 'hardwareApproximate'];
            if (keys.some(key => JSON.stringify(profile[key]) !== JSON.stringify(next[key]))) {
                for (const key of keys) profile[key] = next[key];
                delete profile.smidV2;
                smidV2 = '';
            }
        }
        return { profile, smidV2 };
    }
    return { profile: buildAnchoredProfile(anchor, profilesSpec, gpuPool, seed), smidV2: '' };
}


// Atomic promotion also applies to standalone use; --reset never deletes the
// previous archive before a successful generation has a replacement ready.
function saveArchive(file, archive) {
    fs.mkdirSync(path.dirname(file), { recursive: true });
    const temp = file + '.' + crypto.randomBytes(8).toString('hex') + '.tmp';
    let fd;
    try {
        fd = fs.openSync(temp, 'wx', 0o600);
        fs.writeFileSync(fd, JSON.stringify(archive, null, 2));
        fs.fsyncSync(fd);
        fs.closeSync(fd); fd = undefined;
        fs.renameSync(temp, file);
    } finally {
        if (fd !== undefined) fs.closeSync(fd);
        try { fs.unlinkSync(temp); } catch (error) { if (error.code !== 'ENOENT') throw error; }
    }
}
module.exports = { loadArchive, selectAnchoredArchive, saveArchive };
