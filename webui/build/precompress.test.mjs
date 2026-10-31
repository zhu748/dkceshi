import assert from 'node:assert/strict'
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { randomBytes } from 'node:crypto'
import { gunzipSync } from 'node:zlib'
import test from 'node:test'
import { precompressAssets } from './precompress.mjs'

test('build sidecars round-trip chunks and assets without altering original output', async (t) => {
    const dir = await mkdtemp(join(tmpdir(), 'panel-gzip-'))
    t.after(() => rm(dir, { recursive: true, force: true }))
    await mkdir(join(dir, 'assets'))
    const code = 'console.log("panel");'.repeat(500)
    const css = 'body { color: black; }\n'.repeat(500)
    await writeFile(join(dir, 'assets/main.js'), code)
    await writeFile(join(dir, 'assets/main.css'), css)
    await precompressAssets().writeBundle({ dir }, {
        chunk: { type: 'chunk', fileName: 'assets/main.js', code },
        css: { type: 'asset', fileName: 'assets/main.css', source: new TextEncoder().encode(css) },
        tiny: { type: 'asset', fileName: 'tiny.svg', source: '<svg></svg>' },
        binary: { type: 'asset', fileName: 'photo.png', source: randomBytes(4096) },
    })
    for (const [name, expected] of [['main.js', code], ['main.css', css]]) {
        const gz = await readFile(join(dir, 'assets', name + '.gz'))
        assert.equal(gunzipSync(gz).toString(), expected)
        assert.equal(await readFile(join(dir, 'assets', name), 'utf8'), expected)
        assert.ok(gz.length < expected.length / 2)
    }
    await assert.rejects(readFile(join(dir, 'tiny.svg.gz')), { code: 'ENOENT' })
    await assert.rejects(readFile(join(dir, 'photo.png.gz')), { code: 'ENOENT' })
})
