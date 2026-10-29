'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const { spawnSync } = require('node:child_process');
const { adaptBrowserEnv, VERSION } = require('../../scripts/devid-android-env.cjs');
const { VIVO_SPECS, mobileHardware } = require('../../scripts/devid-android-profile.cjs');
const { selectAnchoredArchive } = require('../../scripts/devid-generate.cjs');

const root = path.resolve(__dirname, '../..');
const assets = path.join(root, 'data/devid');
const anchor = { device_model: 'V2405A', android_version: 15, locale: 'zh-CN', tz: 'Asia/Shanghai', tz_offset: -480 };
const fresh = (a = anchor, seed = 123) => selectAnchoredArchive({ profile: null, smidV2: '' }, a, {}, {}, seed);

test('uses vivo model specifications and preserves identity when locale changes', () => {
    const first = fresh();
    assert.equal(first.profile.androidAdapterVersion, VERSION);
    assert.equal(first.profile.gpu.renderer, 'Immortalis-G925');
    assert.equal(first.profile.screenW, 420);
    const next = selectAnchoredArchive({ ...first, smidV2: 'existing-smid' }, { ...anchor, locale: 'en-US', tz: 'America/Los_Angeles', tz_offset: 420 }, {}, {}, 999);
    assert.equal(next.smidV2, 'existing-smid');
    assert.equal(next.profile.canvasSeed, first.profile.canvasSeed);
    assert.deepEqual(next.profile.lang.list, ['en-US', 'en']);
    assert.equal(next.profile.acceptLanguage, 'en-US,en;q=0.9');
    assert.equal(first.profile.locale, 'zh-CN');
});

test('migrates unversioned or desktop archives and resets smid on model change', () => {
    const first = fresh();
    for (const old of [{ ...first.profile, androidAdapterVersion: undefined }, { ...first.profile, osFamily: 'win' }]) {
        const next = selectAnchoredArchive({ profile: old, smidV2: 'desktop-smid' }, anchor, {}, {}, 999);
        assert.equal(next.smidV2, '');
        assert.notEqual(next.profile.canvasSeed, first.profile.canvasSeed);
    }
    const changed = selectAnchoredArchive({ ...first, smidV2: 'old' }, { ...anchor, device_model: 'V2366GA' }, {}, {}, 123);
    assert.equal(changed.smidV2, '');
    assert.equal(changed.profile.gpu.renderer, 'Adreno (TM) 750');
});

test('no-anchor generation chooses multiple Android models and reuses a saved choice', () => {
    const models = new Set();
    for (let seed = 1; seed <= 60; seed++) {
        const next = fresh(null, seed);
        models.add(next.profile.deviceModel);
        assert.equal(next.profile.osFamily, 'android');
    }
    assert.equal(models.size, 19);
    const first = fresh(null);
    const next = selectAnchoredArchive({ ...first, smidV2: 'same' }, null, {}, {}, 999);
    assert.deepEqual(next, { ...first, smidV2: 'same' });
});

test('unknown models are explicitly approximate and accept mobile hardware overrides', () => {
    const unknown = fresh({ ...anchor, device_model: 'custom-phone' });
    assert.equal(unknown.profile.hardwareSource, 'generic-mobile-template');
    assert.equal(unknown.profile.deviceModel, 'custom-phone');
    const gpu = { vendor: 'ARM', renderer: 'Mali-G715' };
    const custom = fresh({ ...anchor, screen: [400, 900, 2.5], gpu });
    assert.equal(custom.profile.screenW, 400);
    assert.deepEqual(custom.profile.gpu, gpu);
    assert.throws(() => mobileHardware({ ...anchor, gpu: { vendor: 'NVIDIA', renderer: 'ANGLE Direct3D11' } }), /GPU/);
    assert.throws(() => fresh({ ...anchor, device_model: '' }), /device_model/);
    assert.throws(() => fresh({ ...anchor, android_version: 0 }), /android_version/);
    assert.throws(() => fresh({ ...anchor, screen: [-1, 900, 3] }), /screen/);
});

test('unknown legacy source is rejected instead of patching ambiguous text', () => {
    assert.throws(() => adaptBrowserEnv('var profile = {};'), /未知/);
});

test('supplied SDK environment passes real VM checks across model/GPU families and locales', { skip: !fs.existsSync(path.join(assets, 'browser_env.js')), timeout: 30000 }, () => {
    const source = fs.readFileSync(path.join(assets, 'browser_env.js'), 'utf8');
    const adapted = adaptBrowserEnv(source);
    assert.equal(adaptBrowserEnv(adapted), adapted);
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'android-env-'));
    try {
        const cases = [...VIVO_SPECS.map((d) => ({ ...anchor, device_model: d[0], android_version: d[1] })),
            { ...anchor, device_model: 'Pixel 9', android_version: 16, locale: 'en-US', tz: 'America/Los_Angeles', tz_offset: 420 },
            { ...anchor, locale: 'ja-JP', tz: 'Asia/Tokyo', tz_offset: -540 }];
        for (const a of cases) {
            const file = path.join(directory, 'anchor.json');
            fs.writeFileSync(file, JSON.stringify(a));
            const result = spawnSync(process.execPath, ['scripts/devid-generate.cjs', '--selftest', '--anchor', file], { cwd: root, encoding: 'utf8', timeout: 5000 });
            assert.equal(result.status, 0, a.device_model + ' ' + a.locale + ': ' + result.stderr);
            const out = JSON.parse(result.stdout.trim());
            assert.equal(out.ok, true);
            assert.equal(out.pass, out.total);
            assert.ok(out.total >= 88);
        }
    } finally {
        fs.rmSync(directory, { recursive: true });
    }
});
