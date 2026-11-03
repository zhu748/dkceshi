import { dayKey, normalizeUsage, parseDayKey } from './usageUtils.js'

export function aggregateDaily(records) {
    const map = new Map()
    records.forEach((r) => {
        const usage = normalizeUsage(r.usage)
        if (!usage) return
        const key = dayKey(r.created_at || r.createdAt)
        let entry = map.get(key)
        if (!entry) {
            entry = emptyDayEntry(key, parseDayKey(key).getTime())
            map.set(key, entry)
        }
        entry.calls += (r.calls_count || 0)
        entry.prompt += usage.prompt
        entry.output += usage.completion
        entry.reasoning += usage.reasoning
        entry.total += usage.total
        entry.cost += Number(r.cost || 0)

        const model = r.model || 'unknown'
        const caller = String(r.caller_id || r.callerId || 'unknown')
        const account = r.account_id || 'unknown'
        entry.byModel[model] = (entry.byModel[model] || 0) + usage.total
        entry.byModelCost[model] = (entry.byModelCost[model] || 0) + Number(r.cost || 0)
        entry.byCaller[caller] = (entry.byCaller[caller] || 0) + usage.total
        entry.byCallerCost[caller] = (entry.byCallerCost[caller] || 0) + Number(r.cost || 0)
        entry.byAccount[account] = (entry.byAccount[account] || 0) + usage.total
        entry.byAccountCost[account] = (entry.byAccountCost[account] || 0) + Number(r.cost || 0)
    })
    return Array.from(map.values()).sort((a, b) => a.key.localeCompare(b.key))
}

function emptyDayEntry(key, ts) {
    return {
        key,
        ts,
        calls: 0,
        prompt: 0,
        output: 0,
        reasoning: 0,
        total: 0,
        cost: 0,
        byModel: Object.create(null),
        byModelCost: Object.create(null),
        byCaller: Object.create(null),
        byCallerCost: Object.create(null),
        byAccount: Object.create(null),
        byAccountCost: Object.create(null),
    }
}

// mergeDayEntry folds a source day into a target bucket, summing scalar totals
// and every per-model / per-caller channel. Used when a long "all" range is
// bucketed into multi-day points.
function mergeDayEntry(target, source) {
    target.calls += source.calls || 0
    target.prompt += source.prompt || 0
    target.output += source.output || 0
    target.reasoning += source.reasoning || 0
    target.total += source.total || 0
    target.cost += source.cost || 0
    for (const field of ['byModel', 'byModelCost', 'byCaller', 'byCallerCost', 'byAccount', 'byAccountCost']) {
        const src = source[field] || {}
        for (const name of Object.keys(src)) {
            target[field][name] = (target[field][name] || 0) + src[name]
        }
    }
}

// Bound every chart, including a finite range extended by a future-dated CSV
// entry. Buckets keep their first day as the key/label and retain all totals.
const MAX_CHART_POINTS = 180

export function fillDateRange(daily, dateRange) {
    const sorted = (daily || []).slice().sort((a, b) => a.key.localeCompare(b.key))
    const todayKey = dayKey(Date.now())
    const todayStart = parseDayKey(todayKey)
    let start = todayStart
    let end = todayStart

    if (dateRange === 'all') {
        if (sorted.length) {
            start = parseDayKey(sorted[0].key)
            end = parseDayKey(sorted[sorted.length - 1].key)
            if (end.getTime() < todayStart.getTime()) end = todayStart
        }
    } else {
        const days = Math.max(1, Number(dateRange) || 30)
        start = new Date(todayStart.getTime() - (days - 1) * 86400000)
        if (sorted.length) {
            const last = parseDayKey(sorted[sorted.length - 1].key)
            if (last.getTime() > end.getTime()) end = last
        }
    }

    if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) return sorted
    const byKey = new Map(sorted.map((d) => [d.key, d]))
    const out = []
    const last = new Date(end.getTime())

    const totalDays = Math.floor((last.getTime() - start.getTime()) / 86400000) + 1
    if (dateRange === 'all' || totalDays > MAX_CHART_POINTS) {
        const step = Math.max(1, Math.ceil(totalDays / MAX_CHART_POINTS))
        const startMs = start.getTime()
        const bucketMs = step * 86400000
        for (let ts = startMs; ts <= last.getTime(); ts += bucketMs) {
            out.push(emptyDayEntry(dayKey(ts), ts))
        }
        // Visit recorded days once, rather than iterating every missing day
        // between the oldest and newest entry (potentially millions of days).
        for (const day of byKey.values()) {
            const index = Math.floor((parseDayKey(day.key).getTime() - startMs) / bucketMs)
            if (out[index]) mergeDayEntry(out[index], day)
        }
        return out
    }

    const cursor = new Date(start.getTime())
    while (cursor <= last) {
        const key = dayKey(cursor.getTime())
        out.push(byKey.get(key) || emptyDayEntry(key, cursor.getTime()))
        cursor.setUTCDate(cursor.getUTCDate() + 1)
    }
    return out
}
