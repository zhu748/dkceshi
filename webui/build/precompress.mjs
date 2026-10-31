import { writeFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { gzipSync } from 'node:zlib'

// Compress once at build time so the Go server does no per-request compression.
export function precompressAssets() {
    return {
        name: 'precompress-admin-assets',
        apply: 'build',
        async writeBundle(options, bundle) {
            await Promise.all(Object.values(bundle).map(async (file) => {
                if (!/\.(?:js|css|svg|html|json)$/.test(file.fileName)) return
                const source = Buffer.from(file.type === 'chunk' ? file.code : file.source)
                if (source.length < 1024) return
                const compressed = gzipSync(source, { level: 9 })
                if (compressed.length >= source.length) return
                await writeFile(resolve(options.dir, file.fileName + '.gz'), compressed)
            }))
        },
    }
}
