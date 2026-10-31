const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const { mkdtempSync, mkdirSync, copyFileSync, writeFileSync, rmSync } = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { afterEach, test } = require('node:test');

const fixtures = [];
afterEach(() => {
  for (const directory of fixtures.splice(0)) rmSync(directory, { recursive: true, force: true });
});

function fixture() {
  const directory = mkdtempSync(path.join(os.tmpdir(), 'ds2api-line-gate-'));
  fixtures.push(directory);
  mkdirSync(path.join(directory, 'tests/scripts'), { recursive: true });
  copyFileSync(path.join(__dirname, '../scripts/check-refactor-line-gate.sh'), path.join(directory, 'tests/scripts/gate.sh'));
  assert.equal(spawnSync('git', ['init', '--quiet', directory]).status, 0);
  const write = (file, body) => {
    mkdirSync(path.dirname(path.join(directory, file)), { recursive: true });
    writeFileSync(path.join(directory, file), body);
  };
  const lines = (file, count) => write(file, '// line\n'.repeat(count));
  const run = (...args) => spawnSync('bash', [path.join(directory, 'tests/scripts/gate.sh'), ...args], { encoding: 'utf8' });
  return { directory, write, lines, run };
}

test('default discovery includes new source without a plan and enforces its limit', () => {
  const f = fixture();
  f.lines('internal/example.go', 300);
  assert.equal(f.run().status, 0);
  f.lines('internal/example.go', 301);
  const result = f.run();
  assert.equal(result.status, 1);
  assert.match(result.stdout, /OVER internal\/example.go lines=301 limit=300/);
});

test('large legacy files can shrink but cannot grow past their frozen baseline', () => {
  const f = fixture();
  f.write('tests/refactor-line-baseline.tsv', 'internal/legacy.js\t401\n');
  f.lines('internal/legacy.js', 401);
  assert.equal(f.run().status, 0);
  f.lines('internal/legacy.js', 400);
  assert.equal(f.run().status, 0);
  f.lines('internal/legacy.js', 402);
  assert.equal(f.run().status, 1);
});

test('frontend and entrypoint limits apply while all supported test suffixes are excluded', () => {
  const f = fixture();
  f.lines('webui/src/Feature.jsx', 500);
  f.lines('webui/src/App.jsx', 120);
  for (const name of ['feature.test.mjs', 'feature.test.cjs', 'feature.test.tsx', 'feature_test.go']) {
    f.lines(`webui/src/${name}`, 900);
  }
  assert.equal(f.run().status, 0);
  f.lines('webui/src/App.jsx', 121);
  assert.equal(f.run().status, 1);
  f.lines('webui/src/App.jsx', 120);
  f.lines('webui/src/Feature.jsx', 501);
  assert.equal(f.run().status, 1);
});

test('explicit missing targets and empty source inventories fail', () => {
  const f = fixture();
  assert.equal(f.run().status, 1);
  assert.equal(f.run(path.join(f.directory, 'missing-plan.txt')).status, 1);
  f.write('plan.txt', 'internal/missing.go\n');
  const result = f.run(path.join(f.directory, 'plan.txt'));
  assert.equal(result.status, 1);
  assert.match(result.stdout, /MISSING internal\/missing.go/);
});
