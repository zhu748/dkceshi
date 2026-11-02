import test from 'node:test'
import assert from 'node:assert/strict'
import { createRepairModelResource, emptyRepairModelState } from './repairModelResource.js'

const response = models => ({ ok: true, json: async () => ({ models, truncated: false }) })

test('a stale model response cannot replace the latest API protocol selection', async () => {
    const resource = createRepairModelResource()
    const pending = []
    const fetcher = (_url, options) => new Promise(resolve => pending.push({ resolve, options }))
    let state = emptyRepairModelState()
    const commit = patch => { state = { ...state, ...patch } }
    const first = resource.load(fetcher, { protocol: 'openai', base_url: 'https://old.example/v1', api_key: 'old-test-key' }, commit)
    const second = resource.load(fetcher, { protocol: 'gemini', base_url: 'https://new.example/v1beta', api_key: 'new-test-key' }, commit)
    assert.equal(pending[0].options.signal.aborted, true)
    pending[1].resolve(response([{ id: 'new-model' }]))
    await second
    pending[0].resolve(response([{ id: 'old-model' }]))
    await first
    assert.equal(state.models[0].id, 'new-model')
    const body = JSON.parse(pending[1].options.body)
    assert.equal(body.protocol, 'gemini')
    assert.equal(body.api_key, 'new-test-key')
    assert.equal(body.model, undefined)
    assert.equal(body.messages, undefined)
})

test('changing connection details cancels model discovery even if fetch ignores abort', async () => {
    const resource = createRepairModelResource()
    let resolve, commits = 0
    const load = resource.load(() => new Promise(done => { resolve = done }), { protocol: 'claude' }, () => { commits++ })
    resource.cancel()
    resolve(response([{ id: 'stale' }]))
    await load
    assert.equal(commits, 1)
})

test('model discovery errors retain a manual selection workflow', async () => {
    const resource = createRepairModelResource()
    let state = emptyRepairModelState()
    await resource.load(async () => ({ ok: false, json: async () => ({ detail: 'Models endpoint unavailable' }) }), {}, patch => { state = { ...state, ...patch } })
    assert.equal(state.error, 'Models endpoint unavailable')
    assert.equal(state.loading, false)
    assert.equal(state.fetched, false)
})
