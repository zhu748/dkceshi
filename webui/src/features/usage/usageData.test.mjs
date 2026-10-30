import assert from 'node:assert/strict'
import test from 'node:test'

import { aggregateDaily, dayKey, entryToRecord, fillDateRange, sameUsageRecords } from './usageUtils.js'

function entry(overrides = {}) {
    return {
        date: '2026-10-30', model: 'deepseek-v4.1-flash', caller_id: 'caller-a',
        prompt_tokens: 100, completion_tokens: 40, reasoning_tokens: 10,
        total_tokens: 140, calls: 1, cost: 0,
        ...overrides,
    }
}

test('usage polling notices each token counter changing even with unchanged calls and cost', () => {
    const previous = [entryToRecord(entry())]
    assert.equal(sameUsageRecords(previous, [entryToRecord(entry())]), true)
    for (const field of ['prompt_tokens', 'completion_tokens', 'reasoning_tokens', 'total_tokens']) {
        const changed = entry({ [field]: 200 })
        assert.equal(sameUsageRecords(previous, [entryToRecord(changed)]), false, field)
    }
})

test('usage grouping preserves model and caller names matching object properties', () => {
    const names = ['__proto__', 'constructor', 'toString']
    const daily = aggregateDaily(names.map((name) => entryToRecord(entry({ model: name, caller_id: name }))))
    assert.equal(daily.length, 1)
    for (const name of names) {
        assert.equal(daily[0].byModel[name], 140)
        assert.equal(daily[0].byCaller[name], 140)
        assert.equal(daily[0].byModelCost[name], 0)
        assert.equal(daily[0].byCallerCost[name], 0)
    }
    const buckets = fillDateRange(daily, 'all')
    for (const field of ['byModel', 'byCaller']) {
        for (const name of names) {
            assert.equal(buckets.reduce((total, day) => total + (day[field][name] || 0), 0), 140)
        }
    }
})

test('sparse all-time buckets retain every total and empty interval within 180 points', () => {
    const rows = [
        entryToRecord(entry({ date: '1000-01-01' })),
        entryToRecord(entry({ date: '5000-01-01', caller_id: 'caller-b' })),
        entryToRecord(entry({ date: '9999-12-31' })),
    ]
    const buckets = fillDateRange(aggregateDaily(rows), 'all')
    assert.ok(buckets.length <= 180)
    assert.equal(buckets[0].key, '1000-01-01')
    for (const [field, expected] of Object.entries({ prompt: 300, output: 120, reasoning: 30, total: 420, calls: 3 })) {
        assert.equal(buckets.reduce((total, day) => total + day[field], 0), expected, field)
    }
    assert.ok(buckets.some((day) => day.total === 0), 'missing days remain zero-filled')
})

test('future-dated imports cannot expand a finite chart range into thousands of points', () => {
    const today = dayKey(Date.now())
    const future = `${Number(today.slice(0, 4)) + 10}-12-31`
    const rows = [entryToRecord(entry({ date: today })), entryToRecord(entry({ date: future }))]
    const buckets = fillDateRange(aggregateDaily(rows), '30')
    assert.ok(buckets.length <= 180, `unbounded chart: ${buckets.length} points`)
    assert.equal(buckets.reduce((total, day) => total + day.total, 0), 280)
})
