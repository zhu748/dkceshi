'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { checkAssets, REQUIRED } = require('../../scripts/devid-assets.cjs');
const { parseArgs } = require('../../scripts/devid-cli.cjs');
const { loadArchive, saveArchive } = require('../../scripts/devid-state.cjs');

function fixture(t) {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'sdk-assets-'));
    t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
    for (const name of REQUIRED) fs.writeFileSync(path.join(directory, name), name.endsWith('.json') ? '{}' : '/* fixture */');
    return directory;
}

test('asset manifests reject mismatched hashes and invalid SDK syntax', t => {
    const directory = fixture(t), hashes = {};
    for (const name of REQUIRED) hashes['data/devid/' + name] = crypto.createHash('sha256').update(fs.readFileSync(path.join(directory, name))).digest('hex');
    fs.writeFileSync(path.join(directory, 'ANDROID-PACKAGE.json'), JSON.stringify({ format: 1, environment: 'android-web-sdk', adapter_version: 1, native_android_sdk: false, files_sha256: hashes }));
    assert.equal(checkAssets(directory).verified, true);
    fs.writeFileSync(path.join(directory, 'fp-1.min.js'), 'this is invalid JavaScript !!!');
    assert.throws(() => checkAssets(directory), /校验失败/);
    fs.unlinkSync(path.join(directory, 'ANDROID-PACKAGE.json'));
    assert.throws(() => checkAssets(directory), SyntaxError);
});

test('SDK reset does not delete archives before replacement and persistence errors propagate', t => {
    const directory = fixture(t), archive = path.join(directory, 'profile.json');
    const original = '{"version":3,"profile":{"seed":1}}';
    fs.writeFileSync(archive, original);
    assert.deepEqual(loadArchive({ profile: archive, reset: true }), { profile: null, smidV2: '' });
    assert.equal(fs.readFileSync(archive, 'utf8'), original);
    saveArchive(archive, { version: 3, profile: { seed: 2 } });
    assert.equal(JSON.parse(fs.readFileSync(archive)).profile.seed, 2);
    const blocker = path.join(directory, 'blocker');
    fs.writeFileSync(blocker, 'file');
    assert.throws(() => saveArchive(path.join(blocker, 'profile.json'), {}));
    assert.equal(fs.readdirSync(directory).filter(name => name.endsWith('.tmp')).length, 0);
});

test('offline SDK modes reject browser overrides and invalid CLI arguments', () => {
    for (const args of [['--selftest', '--browser'], ['--check-assets', '--via-chromium'], ['--capture', '--browser'], ['--timeout', '0'], ['--timeout', 'Infinity'], ['--unknown']]) {
        assert.throws(() => parseArgs(args));
    }
});

const root = path.resolve(__dirname, '../..'), assets = path.join(root, 'data/devid');
test('SDK capture and selftest never submit network requests or overwrite selftest profiles', { skip: !fs.existsSync(path.join(assets, 'browser_env.js')) }, t => {
    const directory = fixture(t), archive = path.join(directory, 'archive.json'), preload = path.join(directory, 'offline.cjs');
    const original = '{"version":3,"profile":{},"smid_v2":"existing"}';
    fs.writeFileSync(archive, original);
    fs.writeFileSync(preload, "global.fetch = () => { throw new Error('network request during offline SDK mode'); };");
    const run = mode => spawnSync(process.execPath, ['-r', preload, path.join(root, 'scripts/devid-generate.cjs'), mode, '--reset', '--profile', archive, '--timeout', '5'], {
        encoding: 'utf8', timeout: 10000, env: { ...process.env, DS2API_DEVID_DIR: assets },
    });
    const selftest = run('--selftest');
    assert.equal(selftest.status, 0, selftest.stdout + selftest.stderr);
    assert.equal(JSON.parse(selftest.stdout).sdk_payload, true);
    assert.equal(fs.readFileSync(archive, 'utf8'), original);
    const capture = run('--capture');
    assert.equal(capture.status, 0, capture.stdout + capture.stderr);
    const result = JSON.parse(capture.stdout);
    assert.equal(result.mode, 'capture');
    assert.equal(result.device_id, undefined);
    assert.equal(result.report.url, 'https://fp-it-acc.portal101.cn/deviceprofile/v4');
    assert.equal(result.report.headers['sec-ch-ua-platform'], '"Android"');
    assert.equal(JSON.parse(fs.readFileSync(archive)).version, 3);
});
