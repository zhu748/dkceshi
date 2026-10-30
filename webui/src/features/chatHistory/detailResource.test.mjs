import assert from 'node:assert/strict'
import test from 'node:test'

import { createDetailResource } from './detailResource.js'

function deferred() {
    let resolve, reject
    const promise = new Promise((yes, no) => { resolve = yes; reject = no })
    return { promise, resolve, reject }
}

function response(id, etag = id) {
    return { ok: true, status: 200, headers: new Headers({ ETag: etag }), json: async () => ({ item: { id } }) }
}

test('switching details starts immediately and ignores an old response even if abort is ignored', async () => {
    const resource = createDetailResource()
    const requests = []
    const accepted = []
    const fetcher = (url, options) => {
        const request = { url, options, ...deferred() }
        requests.push(request)
        return request.promise
    }
    const first = resource.load('old', fetcher, (item) => accepted.push(item.id))
    const second = resource.load('new', fetcher, (item) => accepted.push(item.id))
    assert.equal(requests.length, 2)
    assert.equal(requests[0].options.signal.aborted, true)
    requests[1].resolve(response('new'))
    await second
    requests[0].resolve(response('old'))
    await first
    assert.deepEqual(accepted, ['new'])
})

test('ETags belong to one detail and an invalidated detail is fetched without a condition', async () => {
    const resource = createDetailResource()
    const headers = []
    const fetcher = async (_url, options) => {
        headers.push(options.headers)
        return headers.length === 2 ? { status: 304 } : response('item', 'revision-1')
    }
    let accepted = 0
    const accept = () => { accepted++ }
    await resource.load('a', fetcher, accept)
    await resource.load('a', fetcher, accept)
    await resource.load('b', fetcher, accept)
    resource.invalidate()
    await resource.load('b', fetcher, accept)
    assert.deepEqual(headers, [{}, { 'If-None-Match': 'revision-1' }, {}, {}])
    assert.equal(accepted, 3)
})

test('polls for the same detail share a request while cancellation suppresses late JSON', async () => {
    const resource = createDetailResource()
    const body = deferred()
    let calls = 0
    let accepted = false
    const fetcher = async () => {
        calls++
        return { ...response('a'), json: () => body.promise }
    }
    const first = resource.load('a', fetcher, () => { accepted = true })
    const second = resource.load('a', fetcher, () => { accepted = true })
    assert.equal(first, second)
    await Promise.resolve()
    resource.cancel()
    body.resolve({ item: { id: 'a' } })
    await first
    assert.equal(calls, 1)
    assert.equal(accepted, false)
})

test('current failures remain visible and superseded failures are ignored', async () => {
    const resource = createDetailResource()
    await assert.rejects(resource.load('a', async () => {
        throw new Error('network down')
    }, () => {}), /network down/)
    const old = deferred()
    const pending = resource.load('a', () => old.promise, () => {})
    await resource.load('b', async () => response('b'), () => {})
    old.reject(new Error('obsolete error'))
    await pending
})
