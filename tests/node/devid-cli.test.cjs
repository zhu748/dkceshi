'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const script = path.resolve(__dirname, '../../scripts/devid-generate.cjs');

for (const relative of [false, true]) {
  test(`SDK CLI diagnoses missing assets in the configured ${relative ? 'relative' : 'absolute'} directory`, t => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'ds2api-sdk-cli-'));
    t.after(() => fs.rmSync(root, { recursive: true, force: true }));
    const assets = path.join(root, 'SDK assets');
    fs.mkdirSync(assets);
    // Presence of this file proves that the overridden directory is used.
    fs.writeFileSync(path.join(assets, 'profiles.json'), '{}');
    const result = spawnSync(process.execPath, [script, '--selftest'], {
      cwd: root,
      encoding: 'utf8',
      timeout: 5000,
      env: { ...process.env, DS2API_DEVID_DIR: relative ? 'SDK assets' : assets },
    });
    assert.equal(result.status, 1);
    const failure = JSON.parse(result.stdout.trim());
    assert.equal(failure.ok, false);
    assert.match(failure.error, /SDK 资源不完整/);
    assert.ok(failure.error.includes(assets));
    assert.match(failure.error, /fp-1\.min\.js/);
    assert.match(failure.error, /browser_env\.js/);
    assert.doesNotMatch(failure.error, /profiles\.json/);
  });
}
