import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { readChatStream as current } from '../../webui/src/features/apiTester/chatStream.js'

// Compare the same public reader, payloads and chunk boundaries. No server or
// upstream account is needed. Run with --expose-gc to reduce carry-over noise.
const root = fileURLToPath(new URL('../../', import.meta.url))
const baseline = process.argv[2] || 'HEAD'
const entryURL = new URL('../../webui/src/features/apiTester/chatStream.js', import.meta.url)
const source = execFileSync('git', ['show', `${baseline}:webui/src/features/apiTester/chatStream.js`], { cwd: root, encoding: 'utf8' })
    .replace(/(\bfrom\s+)(['"])(\.[^'"]+)\2/g, (_, prefix, quote, path) => prefix + JSON.stringify(new URL(path, entryURL).href))
const { readChatStream: previous } = await import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'))
const encoder = new TextEncoder()

function body(bytes, chunkSize) {
    let offset = 0
    return new ReadableStream({ pull(controller) {
        if (offset === bytes.length) { controller.close(); return }
        const end = Math.min(bytes.length, offset + chunkSize)
        controller.enqueue(bytes.subarray(offset, end))
        offset = end
    } })
}

const results = []
for (const [scenario, length, chunkSize] of [
    ['64 KiB event / 256 B chunks', 65536, 256],
    ['1 MiB event / 256 B chunks', 1048576, 256],
    ['512 small events / 4 KiB chunks', 0, 4096],
]) {
    const fragment = 'an independent fragment'
    const event = content => 'data: ' + JSON.stringify({ choices: [{ delta: { content } }] }) + '\n\n'
    const bytes = encoder.encode((length ? event('a'.repeat(length)) : event(fragment).repeat(512)) + 'data: [DONE]\n\n')
    const times = { baseline: [], current: [] }
    for (let run = 0; run < 7; run++) {
        const readers = [['baseline', previous], ['current', current]]
        if (run % 2) readers.reverse()
        for (const [name, read] of readers) {
            globalThis.gc?.()
            const start = performance.now()
            const result = await read(body(bytes, chunkSize), new AbortController().signal, () => {})
            assert.equal(result.content.length, length || 512 * fragment.length)
            if (run > 1) times[name].push(performance.now() - start)
        }
    }
    const median = values => values.sort((a, b) => a - b)[2].toFixed(3)
    results.push({ scenario, baseline_ms: median(times.baseline), current_ms: median(times.current) })
}
console.log(`Baseline: ${baseline}; Node ${process.version}; milliseconds, median of five runs after two warmups.`)
console.table(results)
