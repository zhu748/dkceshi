import test from 'node:test'
import assert from 'node:assert/strict'
import { createRepairTestResource } from './repairTestResource.js'
import { repairDraftBody } from './repairDraft.js'

test('repair test sends the unsaved model and native options without saving configuration', async () => {
    let sent, state
    await createRepairTestResource().run(async (url, options) => {
        sent = { url, body: JSON.parse(options.body) }
        return { ok: true, json: async () => ({ success: true, model: 'draft-model', elapsed_ms: 10 }) }
    }, { mode: 'custom', model: 'draft-model', protocol: 'gemini', extra: '{"generationConfig":{"thinkingConfig":{"includeThoughts":true}}}' }, patch => { state = patch })
    assert.equal(sent.url, '/admin/tool-call-records/repair-test')
    assert.equal(sent.body.model, 'draft-model')
    assert.equal(sent.body.extra_body.generationConfig.thinkingConfig.includeThoughts, true)
    assert.equal(state.result.success, true)
})

test('changed credentials and late JSON cannot overwrite a newer repair test', async () => {
    const resource = createRepairTestResource()
    let resolve, state
    const first = resource.run(async () => ({ ok: true, json: () => new Promise(done => { resolve = done }) }), { model: 'old' }, patch => { state = patch })
    await Promise.resolve()
    resource.cancel()
    await resource.run(async () => ({ ok: true, json: async () => ({ model: 'new' }) }), { model: 'new' }, patch => { state = patch })
    resolve({ model: 'old' })
    await first
    assert.equal(state.result.model, 'new')
})

test('invalid or oversized Unicode extra parameters fail before making an API request', async () => {
    for (const extra of ['[]', 'null', '{', JSON.stringify({ note: '字'.repeat(3000) })]) {
        assert.throws(() => repairDraftBody({ extra }), /invalidExtra/)
    }
    let state
    await createRepairTestResource().run(() => assert.fail('invalid draft reached network'), { extra: '[]' }, patch => { state = patch })
    assert.equal(state.error, 'invalidExtra')
    assert.equal(state.loading, false)
})

test('rapid duplicate repair tests share a single network operation', async () => {
    const resource = createRepairTestResource()
    let resolve, calls = 0
    const fetcher = () => { calls++; return new Promise(done => { resolve = done }) }
    const first = resource.run(fetcher, {}, () => {})
    await resource.run(fetcher, {}, () => {})
    assert.equal(calls, 1)
    resolve({ ok: true, json: async () => ({ success: true }) })
    await first
})
