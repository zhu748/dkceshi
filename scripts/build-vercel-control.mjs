// Build a Linux control executable traced into the Node function. Vercel's Go
// function builder and buildCommand use separate environments, so bootstrap an
// official verified toolchain when `go` is absent from buildCommand's PATH.
import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmod, mkdir, readFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
let go = process.env.DS2API_GO_BINARY || 'go'
const probe = spawnSync(go, ['version'], { encoding: 'utf8' })
const installed = probe.stdout?.match(/go(\d+)\.(\d+)/)
const usable = probe.status === 0 && installed && (Number(installed[1]) > 1 || (Number(installed[1]) === 1 && Number(installed[2]) >= 26))
if (!usable) {
    if (process.platform !== 'linux') throw new Error('Install Go 1.26 or set DS2API_GO_BINARY to build the Vercel control runtime')
    const version = 'go1.26.0'
    const arch = process.arch === 'arm64' ? 'arm64' : 'amd64'
    const base = resolve(root, '.tmp/vercel-go')
    await mkdir(base, { recursive: true })
    // curl preserves the build environment's proxy and CA configuration.
    const metadata = spawnSync('curl', ['-fsSL', '--max-time', '30', 'https://go.dev/dl/?mode=json&include=all'], { encoding: 'utf8', maxBuffer: 16 << 20 })
    if (metadata.status !== 0) throw new Error('Go release metadata download failed')
    const release = JSON.parse(metadata.stdout).find(item => item.version === version)
    const archive = release?.files.find(item => item.os === 'linux' && item.arch === arch && item.kind === 'archive')
    if (!archive) throw new Error(`Official ${version} archive not found`)
    const tarball = resolve(base, archive.filename)
    const download = spawnSync('curl', ['-fsSL', '--max-time', '120', `https://go.dev/dl/${archive.filename}`, '-o', tarball], { stdio: 'inherit' })
    if (download.status !== 0) throw new Error('Go toolchain download failed')
    const bytes = await readFile(tarball)
    if (createHash('sha256').update(bytes).digest('hex') !== archive.sha256) throw new Error('Go toolchain checksum mismatch')
    if (spawnSync('tar', ['-xzf', tarball, '-C', base], { stdio: 'inherit' }).status !== 0) throw new Error('Go toolchain extraction failed')
    go = resolve(base, 'go/bin/go')
}
await mkdir(resolve(root, 'internal/runtime'), { recursive: true })
const result = spawnSync(go, ['build', '-trimpath', '-ldflags=-s -w', '-o', 'internal/runtime/vercel-control', './cmd/vercel-control'], {
    cwd: root, stdio: 'inherit', env: { ...process.env, GOOS: 'linux', GOARCH: 'amd64', CGO_ENABLED: '0' },
})
if (result.status !== 0) throw new Error('Vercel control build failed')

await chmod(resolve(root, 'internal/runtime/vercel-control'), 0o755)
