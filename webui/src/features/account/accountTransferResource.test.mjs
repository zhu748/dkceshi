import assert from 'node:assert/strict'
import test from 'node:test'
import { createAccountTransferResource, MAX_ACCOUNT_IMPORT_BYTES } from './accountTransferResource.js'

function deferred() {
    let resolve
    const promise = new Promise(done => { resolve = done })
    return { promise, resolve }
}
const response = body => new Response(JSON.stringify(body), {headers: {'content-type': 'application/json'}})
const backup = () => new Blob(['{"accounts":[{"email":"a@x.com","password":"原文","disabled":true,"hif_dliq_expires_at":9007199254740993}]}'], {type: 'application/json'})

test('export downloads the complete server blob instead of the paginated masked list', async () => {
    const expected = backup(), busy = [], events = []
    const resource = createAccountTransferResource(value => busy.push(value))
    await resource.exportAccounts(async (url, options) => {
        assert.equal(url, '/admin/accounts/export')
        assert.equal(options.signal.aborted, false)
        return {ok: true, blob: async () => expected}
    }, actual => {assert.equal(actual, expected); events.push('download')}, {onExported: () => events.push('success')})
    assert.deepEqual(busy, ['export', ''])
    assert.deepEqual(events, ['download', 'success'])
})

test('import sends original file bytes and refreshes only after confirmed success', async () => {
    const file = backup(), busy = [], events = []
    const resource = createAccountTransferResource(value => busy.push(value))
    await resource.importAccounts(file, async (url, options) => {
        assert.equal(url, '/admin/accounts/import')
        assert.equal(options.method, 'POST')
        assert.equal(options.body, file)
        assert.equal(options.headers['Content-Type'], 'application/json')
        assert.ok((await options.body.text()).includes('9007199254740993'))
        return response({success: true, added: 2, updated: 1})
    }, {onImported: (data, isCurrent) => {assert.equal(isCurrent(), true); events.push(data)}})
    assert.deepEqual(busy, ['import', ''])
    assert.deepEqual(events, [{success: true, added: 2, updated: 1}])
})

test('duplicate clicks and concurrent import/export share one transfer', async () => {
    const pending = deferred(), resource = createAccountTransferResource(() => {})
    let sends = 0
    const fetcher = async () => {sends++; return pending.promise}
    const first = resource.importAccounts(backup(), fetcher)
    await resource.importAccounts(backup(), fetcher)
    await resource.exportAccounts(fetcher, () => assert.fail('unexpected download'))
    assert.equal(sends, 1)
    pending.resolve(response({success: true}))
    await first
})

test('canceled response, JSON and blob completions cannot notify or download', async () => {
    for (const stage of ['response', 'json', 'blob']) {
        const pending = deferred(), events = []
        let signal
        const resource = createAccountTransferResource(value => events.push(value))
        const fetcher = async (_url, options) => {
            signal = options.signal
            if (stage === 'response') return pending.promise
            return {ok: true, json: () => pending.promise, blob: () => pending.promise}
        }
        const callbacks = {onError: () => events.push('error'), onImported: () => events.push('imported'), onExported: () => events.push('exported')}
        const request = stage === 'blob' ? resource.exportAccounts(fetcher, () => events.push('download'), callbacks) : resource.importAccounts(backup(), fetcher, callbacks)
        await Promise.resolve()
        await Promise.resolve()
        resource.cancel()
        assert.equal(signal.aborted, true)
        const before = events.length
        pending.resolve(stage === 'response' ? response({success: true}) : stage === 'json' ? {success: true} : backup())
        await request
        assert.equal(events.length, before)
    }
})

test('file size and server validation errors preserve the current account page', async () => {
    const events = [], resource = createAccountTransferResource(() => {})
    await resource.importAccounts({size: MAX_ACCOUNT_IMPORT_BYTES + 1}, () => assert.fail('oversized request sent'), {onTooLarge: () => events.push('too large')})
    await resource.importAccounts(backup(), async () => new Response('{"detail":"invalid state"}', {status: 400}), {
        onError: detail => events.push(detail), onImported: () => assert.fail('invalid import refreshed the page'),
    })
    assert.deepEqual(events, ['too large', 'invalid state'])
})
