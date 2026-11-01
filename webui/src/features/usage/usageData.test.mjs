import assert from 'node:assert/strict'
import test from 'node:test'

import { aggregateDaily, dayKey, entryToRecord, fillDateRange, sameUsageRecords } from './usageUtils.js'
import { accountDisplay, accountOptionsFor, buildUsageSeries, filterUsageRecords, groupUsage, usageTotals } from './usageDashboard.js'

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


test('account and model breakdowns conserve calls and tokens without adding reasoning twice', () => {
    const rows = [
        entryToRecord(entry({ account_id: 'account:a', account_label: 'A', calls: 2, last_used_at: 1000 })),
        entryToRecord(entry({ account_id: 'account:a', account_label: 'New A', model: 'other', last_used_at: 2000 })),
        entryToRecord(entry({ account_id: 'account:b', calls: 3 })),
        entryToRecord(entry()),
    ]
    for (const dimension of ['account', 'model']) {
        const grouped = groupUsage(rows, dimension)
        assert.equal(grouped.reduce((sum, row) => sum + row.calls, 0), 7)
        assert.equal(grouped.reduce((sum, row) => sum + row.total, 0), 560)
        assert.equal(grouped.reduce((sum, row) => sum + row.reasoning, 0), 40)
        assert.equal(grouped.reduce((sum, row) => sum + row.share, 0), 1)
    }
    const account = groupUsage(rows, 'account').find((row) => row.id === 'account:a')
    assert.equal(account.label, 'New A')
    assert.equal(account.lastUsedAt, 2000)
    assert.equal(usageTotals(rows).total, 560)
    assert.equal(groupUsage([], 'account').length, 0)
})

test('date, model and account filters intersect at the GMT+8 day boundary', () => {
    const rows = ['2026-10-24', '2026-10-25', '2026-10-31', '2026-11-01'].flatMap((date) => [
        entryToRecord(entry({ date, account_id: 'account:a' })),
        entryToRecord(entry({ date, account_id: 'account:b', model: 'other' })),
    ])
    const now = Date.parse('2026-10-30T16:00:01Z') // Oct 31 in GMT+8
    const filtered = filterUsageRecords(rows, { dateRange: '7', account: 'account:a', model: 'deepseek-v4.1-flash' }, now)
    assert.deepEqual(filtered.map((row) => row.date), ['2026-10-25', '2026-10-31'])
    assert.equal(filterUsageRecords(rows, { account: 'unknown' }, now).length, 0)
    assert.equal(filterUsageRecords(rows, {}, now).length, 8)
})

test('polling observes account attribution, label and last-used changes; invalid timestamps are unavailable', () => {
    const previous = [entryToRecord(entry({ account_id: 'account:a', account_label: 'A', last_used_at: 1000 }))]
    for (const update of [{ account_id: 'account:b' }, { account_label: 'B' }, { last_used_at: 2000 }]) {
        assert.equal(sameUsageRecords(previous, [{ ...previous[0], ...update }]), false)
    }
    for (const value of [Number.MAX_VALUE, -1, 'not-a-time']) {
        assert.equal(entryToRecord(entry({ last_used_at: value })).last_used_at, 0)
    }
})

test('account chart top-eight and Other preserve totals and prototype-like identifiers', () => {
    const rows = Array.from({ length: 12 }, (_, index) => entryToRecord(entry({
        account_id: index === 0 ? '__proto__' : `account:${index}`,
        cost: index + 1,
    })))
    const t = (key) => key
    const daily = fillDateRange(aggregateDaily(rows), 'all')
    const accounts = accountOptionsFor(rows, t)
    const series = buildUsageSeries(daily, 'account', accounts, t)
    assert.equal(series.length, 9)
    assert.equal(daily.reduce((total, day) => total + series.reduce((n, item) => n + item.valueOf(day), 0), 0), 1680)
    assert.equal(daily.reduce((total, day) => total + series.reduce((n, item) => n + item.valueOfCost(day), 0), 0), 78)
    assert.equal(new Set(series.map((item) => item.name)).size, 9)
    assert.equal(buildUsageSeries(daily, 'account', accounts, t, true)[0].fullName, 'account:11')
    assert.equal(accountDisplay('unknown', '', t), 'usage.unknownAccount')
    assert.match(accountDisplay('direct:caller:abc123', '', t), /usage.directToken/)
})
