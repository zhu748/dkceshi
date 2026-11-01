// Guards the CSV round-trip used by the Token usage Export / Import buttons.
//
// The import feature is only useful if it accepts exactly what buildUsageCsv
// emits: a UTF-8 BOM, the fixed header, quoted fields and GMT+8 day keys. These
// tests pin that contract plus the bad-row handling, because a silent mismatch
// would merge garbage into the ledger.
//
// Run with the rest of the Node unit tests via tests/scripts/run-unit-node.sh.
import assert from 'node:assert/strict'
import test from 'node:test'

import { buildUsageCsv, parseUsageCsv } from './usageCsv.js'

function record(overrides) {
    return {
        model: 'deepseek-v4.1-flash',
        caller_id: 'caller-a',
        date: '2026-09-17',
        calls_count: 3,
        cost: 0.5,
        // Shape produced by entryToRecord / consumed by normalizeUsage.
        usage: {
            prompt_tokens: 100,
            completion_tokens: 40,
            total_tokens: 140,
            completion_tokens_details: { reasoning_tokens: 10 },
        },
        ...overrides,
    }
}

test('round-trips buildUsageCsv output through parseUsageCsv', () => {
    const records = [
        record({}),
        record({ model: 'deepseek-v4.1-flash-search', caller_id: 'caller,b', date: '2026-09-18', cost: 1.25 }),
    ]
    const { csv, count } = buildUsageCsv(records)
    assert.equal(count, 2)

    const { entries, skipped, error } = parseUsageCsv(csv)
    assert.equal(error, null)
    assert.equal(skipped, 0)
    assert.equal(entries.length, 2)
    assert.deepEqual(entries[0], {
        date: '2026-09-17',
        model: 'deepseek-v4.1-flash',
        caller_id: 'caller-a',
        prompt_tokens: 100,
        completion_tokens: 40,
        reasoning_tokens: 10,
        total_tokens: 140,
        calls: 3,
        cost: 0.5,
    })
    assert.equal(entries[1].caller_id, 'caller,b')
})

test('accepts a BOM-less header and tolerates extra columns', () => {
    const csv = 'note,model,date,caller_id,prompt_tokens,completion_tokens,calls,cost\n' +
        'x,deepseek-v4.1-flash,2026-09-19,caller-c,10,5,1,0.01\n'
    const { entries, error } = parseUsageCsv(csv)
    assert.equal(error, null)
    assert.equal(entries.length, 1)
    assert.equal(entries[0].total_tokens, 15)
    assert.equal(entries[0].reasoning_tokens, 0)
})

test('reports an unrecognized header instead of importing it', () => {
    const { entries, error } = parseUsageCsv('foo,bar\n1,2\n')
    assert.equal(entries.length, 0)
    assert.equal(error, 'usage.importInvalidHeader')
})

test('reports an empty file', () => {
    assert.deepEqual(parseUsageCsv(''), { entries: [], skipped: 0, error: 'usage.importEmpty' })
    assert.deepEqual(parseUsageCsv('\uFEFF'), { entries: [], skipped: 0, error: 'usage.importEmpty' })
})

test('skips rows with a bad date or missing model and reports them', () => {
    const csv = 'model,caller_id,date,prompt_tokens,completion_tokens,total_tokens,calls,cost\n' +
        'deepseek-v4.1-flash,caller-a,not-a-date,10,5,15,1,0\n' +
        ',caller-a,2026-09-19,10,5,15,1,0\n' +
        'deepseek-v4.1-flash,caller-a,2026-09-19,10,5,15,1,0\n'
    const { entries, skipped, error } = parseUsageCsv(csv)
    assert.equal(error, null)
    assert.equal(entries.length, 1)
    assert.equal(skipped, 2)
})

test('reports no valid rows when every row is bad', () => {
    const csv = 'model,caller_id,date,prompt_tokens,completion_tokens\n' +
        'deepseek-v4.1-flash,caller-a,not-a-date,10,5\n'
    const { entries, error, skipped } = parseUsageCsv(csv)
    assert.equal(entries.length, 0)
    assert.equal(skipped, 1)
    assert.equal(error, 'usage.importNoRows')
})

test('parses quoted fields containing commas and escaped quotes', () => {
    const csv = 'model,caller_id,date,prompt_tokens,completion_tokens,calls,cost\n' +
        '"model,quoted","caller ""x""",2026-09-19,10,5,1,0\n'
    const { entries, error } = parseUsageCsv(csv)
    assert.equal(error, null)
    assert.equal(entries[0].model, 'model,quoted')
    assert.equal(entries[0].caller_id, 'caller "x"')
})


test('account attribution and last-used metadata survive a CSV round trip', () => {
    const last = Date.parse('2026-10-31T12:34:00Z')
    const { csv } = buildUsageCsv([record({ account_id: 'account:abc', account_label: 'Main, "A"', last_used_at: last })])
    const { entries, error } = parseUsageCsv(csv)
    assert.equal(error, null)
    assert.equal(entries[0].account_id, 'account:abc')
    assert.equal(entries[0].account_label, 'Main, "A"')
    assert.equal(entries[0].last_used_at, last)
})

test('CSV preserves sub-cent cost precision across export and import', () => {
    for (const cost of [0.00000037, 0.1234567891234567]) {
        const { csv } = buildUsageCsv([record({ cost })])
        assert.equal(parseUsageCsv(csv).entries[0].cost, cost)
    }
})

test('CSV rejects invalid numeric rows and impossible calendar dates', () => {
    const header = 'model,date,prompt_tokens,completion_tokens,reasoning_tokens,total_tokens,calls,cost\n'
    for (const row of [
        'model,2026-02-30,10,5,0,15,1,0',
        'model,2026-11-01,-10,5,0,15,1,0',
        'model,2026-11-01,oops,5,0,15,1,0',
        'model,2026-11-01,1.5,5,0,15,1,0',
        'model,2026-11-01,10,5,0,1,1,0',
        'model,2026-11-01,10,5,6,15,1,0',
        'model,2026-11-01,10,5,0,15,1,-1',
    ]) {
        const result = parseUsageCsv(header + row)
        assert.equal(result.entries.length, 0, row)
        assert.equal(result.skipped, 1, row)
    }
})

test('CSV rejects an unterminated quoted field instead of silently importing it', () => {
    const result = parseUsageCsv('model,date,prompt_tokens,completion_tokens\nmodel,2026-11-01,10,"5')
    assert.equal(result.entries.length, 0)
    assert.ok(result.error)
})

test('spreadsheet formulas are escaped reversibly without altering legacy apostrophes', () => {
    for (const label of ['=HYPERLINK("https://example.invalid")', '+SUM(1,2)', '@SUM(1)', '\t=1+1', "'name"]) {
        const { csv } = buildUsageCsv([record({ model: '=1+1', account_label: label })])
        assert.ok(csv.includes("\n'=1+1,"), 'formula-looking model exported as an executable cell')
        const parsed = parseUsageCsv(csv)
        assert.equal(parsed.entries[0].model, '=1+1')
        assert.equal(parsed.entries[0].account_label, label)
    }
    const legacy = parseUsageCsv("model,date,prompt_tokens,completion_tokens\n'model,2026-11-01,1,1")
    assert.equal(legacy.entries[0].model, "'model")
})
