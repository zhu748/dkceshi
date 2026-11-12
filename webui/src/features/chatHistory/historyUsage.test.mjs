import test from 'node:test'
import assert from 'node:assert/strict'
import { historyUsage, formatTokenCount } from './historyUsage.js'

test('old and pending records distinguish missing counts from real zero', () => {
    assert.equal(historyUsage({ status: 'streaming' }).pending, true)
    const old = historyUsage({ usage: { total_tokens: 9 } })
    assert.equal(old.input, null)
    assert.equal(old.total, 9)
    assert.equal(old.source, 'unknown')
    assert.equal(formatTokenCount(null), '—')
    assert.equal(formatTokenCount(0), '0')
    assert.equal(historyUsage({ usage: { prompt_tokens: -1, completion_tokens: 1.5 } }).output, null)
})

test('history list summaries and protocol detail usage show the same counts', () => {
    const detail = historyUsage({ usage: { input_tokens: 11, output_tokens: 29, total_tokens: 40, reasoning_tokens: 9, ds2api: { source: 'upstream', input_source: 'upstream', output_source: 'upstream', complete: false, upstream: { messages: [{ message_id: 2, accumulated_tokens: 2551 }] } } } })
    const summary = historyUsage({ usage: { prompt_tokens: 11, completion_tokens: 29, total_tokens: 40, reasoning_tokens: 9, source: 'upstream', complete: false, upstream_accumulated_tokens: 2551 } })
    for (const key of ['input', 'output', 'total', 'reasoning', 'source', 'complete', 'accumulated']) assert.equal(summary[key], detail[key], key)
    assert.equal(historyUsage({ usage: { reasoning_tokens: 0, ds2api: { reasoning_source: 'unknown' } } }).reasoning, null)
})
