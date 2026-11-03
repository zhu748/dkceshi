// Usage: node tests/tools/bench-usage-data.mjs [path/to/usageUtils.js]
// The optional path allows the same fixture to benchmark a previous revision.
import assert from 'node:assert/strict'
import { performance } from 'node:perf_hooks'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const target = process.argv[2]
    ? pathToFileURL(resolve(process.argv[2])).href
    : new URL('../../webui/src/features/usage/usageUtils.js', import.meta.url).href
const { entryToRecord } = await import(target)
const { aggregateDaily, fillDateRange } = process.argv[2]
    ? await import(target)
    : await import('../../webui/src/features/usage/usageAggregation.js')
const daily = aggregateDaily(['1000-01-01', '5000-01-01', '9999-12-31'].map((date) => entryToRecord({
    date, model: 'model', caller_id: 'caller', prompt_tokens: 100,
    completion_tokens: 40, total_tokens: 140, calls: 1,
})))
const durations = []
for (let iteration = 0; iteration < 6; iteration++) {
    const start = performance.now()
    const buckets = fillDateRange(daily, 'all')
    const duration = performance.now() - start
    assert.ok(buckets.length <= 180)
    assert.equal(buckets.reduce((total, day) => total + day.total, 0), 420)
    if (iteration > 0) durations.push(duration)
}
durations.sort((a, b) => a - b)
console.log(JSON.stringify({ fixture: 'sparse-9000-years', runs: durations.length,
    median_ms: durations[Math.floor(durations.length / 2)], min_ms: durations[0], max_ms: durations.at(-1) }))

// Keep conversion outside the timer to isolate aggregation cost. Repeated days
// reflect the ledger's per-model/caller/account rows, not individual requests.
const records = Array.from({ length: 100000 }, (_, i) => entryToRecord({
    date: `2026-10-${String(1 + i % 30).padStart(2, '0')}`, model: `model-${i % 4}`,
    account_id: `account-${i % 200}`, prompt_tokens: 100,
    completion_tokens: 40, total_tokens: 140, calls: 1,
}))
for (let i = 0; i < 3; i++) aggregateDaily(records)
const denseDurations = []
for (let i = 0; i < 7; i++) {
    const start = performance.now()
    const result = aggregateDaily(records)
    denseDurations.push(performance.now() - start)
    assert.equal(result.length, 30)
    assert.equal(result.reduce((sum, day) => sum + day.total, 0), 14000000)
}
denseDurations.sort((a, b) => a - b)
console.log(JSON.stringify({ fixture: '100k-records-30-days', runs: denseDurations.length,
    median_ms: denseDurations[3], min_ms: denseDurations[0], max_ms: denseDurations.at(-1) }))
