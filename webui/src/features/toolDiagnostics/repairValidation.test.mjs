import test from 'node:test'
import assert from 'node:assert/strict'
import { repairDraftBody } from './repairDraft.js'
import { createRepairTestResource } from './repairTestResource.js'
import { createRepairModelResource } from './repairModelResource.js'

test('repair settings reject blank, fractional and out-of-range numeric controls', () => {
 for (const retry_count of ['', -1, 11, 1.5, NaN, Infinity]) assert.throws(() => repairDraftBody({ retry_count }), /invalidRetries/)
 for (const timeout_seconds of ['', 0, 301, 1.5, NaN, Infinity]) assert.throws(() => repairDraftBody({ timeout_seconds }), /invalidTimeout/)
 const body = repairDraftBody({ retry_count: 0, timeout_seconds: 1 })
 assert.equal(body.retry_count, 0)
 assert.equal(body.timeout_seconds, 1)
})

test('invalid retry controls fail locally without sending a repair probe', async () => {
 let requests = 0, state
 await createRepairTestResource().run(async () => { requests++; return { ok: true, json: async () => ({ success: true }) } }, { retry_count: '' }, value => { state = value })
 assert.equal(requests, 0)
 assert.equal(state.error, 'invalidRetries')
})

test('model discovery validates its timeout before sending a request', async () => {
 let requests = 0, state
 await createRepairModelResource().load(async () => { requests++; return { ok: true, json: async () => ({ models: [] }) } }, { timeout_seconds: '' }, patch => { state = patch })
 assert.equal(requests, 0)
 assert.equal(state.error, 'invalidTimeout')
})
