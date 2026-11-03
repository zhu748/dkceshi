import test from 'node:test'
import assert from 'node:assert/strict'
import { canReuseRepairKey, repairDraftBody, repairThinkingModes } from './repairDraft.js'

test('repair draft preserves zero retries and native thinking controls', () => {
 const body = repairDraftBody({ retry_count: 0, reasoning_effort: 'high', thinking_mode: 'budget', extra: '{"temperature":0.2}' })
 assert.equal(body.retry_count, 0)
 assert.equal(body.reasoning_effort, 'high')
 assert.equal(body.thinking_mode, 'budget')
 assert.deepEqual(body.extra_body, { temperature: 0.2 })
})

test('saved key indicator follows the current protocol, origin and clear choice', () => {
 const settings = { has_api_key: true, protocol: 'openai', base_url: 'https://example.test/v1' }
 const draft = { protocol: 'openai', base_url: 'https://example.test:443/another' }
 assert.equal(canReuseRepairKey(settings, draft), true)
 for (const patch of [{ protocol: 'claude' }, { base_url: 'http://example.test' }, { base_url: 'https://another.test' }, { clear_api_key: true }]) assert.equal(canReuseRepairKey(settings, { ...draft, ...patch }), false)
 assert.deepEqual(repairThinkingModes('gemini'), ['auto', 'level', 'budget'])
 assert.deepEqual(repairThinkingModes('openai'), [])
})
