import test from 'node:test'
import assert from 'node:assert/strict'
import { createChatRequest } from './chatRequest.js'
import { readChatStream } from './chatStream.js'
import { createRequestGroup } from '../../utils/latestRequest.js'

const deferred = () => {
    let resolve, reject
    const promise = new Promise((yes, no) => { resolve = yes; reject = no })
    return { promise, resolve, reject }
}
function harness() {
    const calls = [], state = {}, messages = []
    const setters = Object.fromEntries(['Loading', 'IsStreaming', 'Response', 'StreamingContent', 'StreamingThinking'].map(key =>
        ['set' + key, value => { state[key] = typeof value === 'function' ? value(state[key]) : value }]))
    const client = createChatRequest(setters, (url, options) => {
        const pending = deferred()
        calls.push({ ...pending, url, signal: options.signal })
        return pending.promise
    })
    const options = { t: key => key, onMessage: (...args) => messages.push(args), model: 'm', message: 'hello', effectiveKey: 'key', streamingMode: false }
    return { calls, state, messages, client, options }
}

test('a late aborted fetch cannot clear or overwrite the next API test', async () => {
    const h = harness(), first = h.client.run(h.options)
    assert.equal(h.client.stop(), true)
    const second = h.client.run(h.options)
    h.calls[0].reject(new DOMException('Aborted', 'AbortError'))
    await first
    assert.equal(h.state.Loading, true)
    assert.equal(h.calls[1].signal.aborted, false)
    assert.equal(h.client.stop(), true)
    assert.equal(h.calls[1].signal.aborted, true)
    h.calls[1].resolve(new Response('{}'))
    await second
    assert.equal(h.messages.length, 0)
})

test('a late JSON body cannot overwrite a newer response', async () => {
    const h = harness(), json = deferred()
    const first = h.client.run(h.options)
    h.calls[0].resolve({ ok: true, status: 200, json: () => json.promise })
    await Promise.resolve()
    const second = h.client.run(h.options)
    h.calls[1].resolve(new Response('{"answer":"new"}'))
    await second
    json.resolve({ answer: 'old' })
    await first
    assert.equal(h.state.Response.answer, 'new')
    assert.equal(h.messages.length, 1)
})

test('unmount cancellation suppresses late errors and UI updates', async () => {
    const h = harness(), pending = h.client.run(h.options)
    h.client.cancel()
    const before = structuredClone(h.state)
    h.calls[0].reject(new Error('late failure'))
    await pending
    assert.deepEqual(h.state, before)
    assert.equal(h.messages.length, 0)
})

function stream(chunks) {
    return new ReadableStream({ start(controller) {
        for (const chunk of chunks) controller.enqueue(chunk)
        controller.close()
    } })
}

test('SSE handles split UTF-8, CRLF, multiline data and preserves finish reason', async () => {
    const text = 'data: {"choices":[{"delta":{"content":"中文"}}]}\r\n\r\ndata:{"choices":\n' +
        'data: [{"delta":{},"finish_reason":"length"}]}\n\ndata: [DONE]'
    const bytes = new TextEncoder().encode(text), deltas = []
    const result = await readChatStream(stream([...bytes].map(byte => Uint8Array.of(byte))), new AbortController().signal, delta => deltas.push(delta))
    assert.equal(result.content, '中文')
    assert.equal(result.finishReason, 'length')
    assert.equal(deltas[0].content, '中文')
})

test('a truncated stream fails instead of being reported as completed', async () => {
    await assert.rejects(readChatStream(stream([new TextEncoder().encode('data: {"choices":[{"delta":{"content":"partial"}}]}\n\n')]), new AbortController().signal, () => {}), /before completion/)
})

test('stream errors retain their status and error code', async () => {
    const bytes = new TextEncoder().encode('data: {"status_code":429,"error":{"code":"rate_limit","message":"busy"}}\n\n')
    await assert.rejects(readChatStream(stream([bytes]), new AbortController().signal, () => {}), error => error.message === 'busy' && error.statusCode === 429 && error.code === 'rate_limit')
})

test('request groups reject duplicates and isolate old completion after reopen', () => {
    const group = createRequestGroup(), old = group.start('same')
    assert.equal(group.start('same'), null)
    group.cancel()
    const current = group.start('same')
    old.finish()
    assert.equal(old.isCurrent(), false)
    assert.equal(current.isCurrent(), true)
    assert.equal(group.size, 1)
    current.finish()
    assert.equal(group.size, 0)
})
