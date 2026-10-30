// Usage: node tests/tools/bench-usage-data.mjs [path/to/usageUtils.js]
// The optional path allows the same fixture to benchmark a previous revision.
import assert from 'node:assert/strict'
import { performance } from 'node:perf_hooks'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const target = process.argv[2]
    ? pathToFileURL(resolve(process.argv[2])).href
    : new URL('../../webui/src/features/usage/usageUtils.js', import.meta.url).href
const { aggregateDaily, entryToRecord, fillDateRange } = await import(target)
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
