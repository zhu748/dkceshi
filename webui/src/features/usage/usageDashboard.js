import { dayKey, normalizeUsage, parseDayKey, shortCaller } from './usageUtils.js'

const PALETTES = {
    model: ['#FF8800', '#FF5500', '#FFAA33', '#E64400', '#FFB84D', '#F97316', '#C2410C', '#FDBA74'],
    account: ['#10B981', '#0D9488', '#14B8A6', '#059669', '#34D399', '#0891B2', '#06B6D4', '#6EE7B7'],
    apikey: ['#A0DCFD', '#60B3FE', '#0C70F3', '#4DA3FF', '#1F6FEB', '#38BDF8', '#7DD3FC', '#1D4ED8'],
}

export function accountDisplay(id, label, t) {
    if (!id || id === 'unknown') return t('usage.unknownAccount')
    const suffix = id.slice(-6)
    if (id.startsWith('direct:')) return `${t('usage.directToken')} · ${suffix}`
    return `${label || t('usage.account')} · ${suffix}`
}

export function accountOptionsFor(records, t) {
    return groupUsage(records, 'account').map((row) => ({
        id: row.id,
        label: accountDisplay(row.id, row.label, t),
    })).sort((a, b) => a.label.localeCompare(b.label))
}

export function filterUsageRecords(records, { dateRange = 'all', model = 'all', account = 'all' } = {}, now = Date.now()) {
    const today = parseDayKey(dayKey(now)).getTime()
    const start = today - (Math.max(1, Number(dateRange) || 30) - 1) * 86400000
    return records.filter((r) => {
        if (model !== 'all' && (r.model || 'unknown') !== model) return false
        if (account !== 'all' && (r.account_id || 'unknown') !== account) return false
        if (dateRange === 'all') return true
        const ts = Number(r.created_at || r.createdAt || 0)
        return ts > 0 && ts >= start && ts < today + 86400000
    })
}

export function usageTotals(records) {
    return records.reduce((sum, r) => {
        const u = normalizeUsage(r.usage)
        if (!u) return sum
        sum.calls += Number(r.calls_count || 0)
        sum.prompt += u.prompt
        sum.output += u.completion
        sum.reasoning += u.reasoning
        sum.total += u.total
        sum.cost += Number(r.cost || 0)
        return sum
    }, { calls: 0, prompt: 0, output: 0, reasoning: 0, total: 0, cost: 0 })
}

export function groupUsage(records, dimension) {
    const groups = new Map()
    for (const r of records) {
        const u = normalizeUsage(r.usage)
        if (!u) continue
        const id = dimension === 'account' ? (r.account_id || 'unknown') : (r.model || 'unknown')
        let row = groups.get(id)
        if (!row) {
            row = { id, label: '', calls: 0, prompt: 0, output: 0, reasoning: 0, total: 0, cost: 0, lastUsedAt: 0 }
            groups.set(id, row)
        }
        row.calls += Number(r.calls_count || 0)
        row.prompt += u.prompt
        row.output += u.completion
        row.reasoning += u.reasoning
        row.total += u.total
        row.cost += Number(r.cost || 0)
        const at = Number(r.last_used_at || 0)
        if (at >= row.lastUsedAt) {
            row.lastUsedAt = at
            if (r.account_label) row.label = r.account_label
        }
    }
    const rows = Array.from(groups.values())
    const total = rows.reduce((sum, row) => sum + row.total, 0)
    return rows.map((row) => ({ ...row, share: total > 0 ? row.total / total : 0 }))
        .sort((a, b) => b.total - a.total || a.id.localeCompare(b.id))
}

export function buildUsageSeries(daily, dimension, accounts, t, costMode = false) {
    const field = { model: 'byModel', account: 'byAccount', apikey: 'byCaller' }[dimension] || 'byModel'
    const costField = `${field}Cost`
    const labels = new Map(accounts.map((item) => [item.id, item.label]))
    const totals = new Map()
    for (const day of daily) {
        for (const [id, value] of Object.entries(day[costMode ? costField : field] || {})) {
            totals.set(id, (totals.get(id) || 0) + value)
        }
    }
    const sorted = Array.from(totals).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    const groups = sorted.slice(0, 8).map(([id]) => [id])
    if (sorted.length > 8) groups.push(sorted.slice(8).map(([id]) => id))
    const usedNames = new Set()
    return groups.map((ids, index) => {
        const other = index === 8
        const id = ids[0]
        const base = other ? t('usage.other') : dimension === 'account'
            ? (labels.get(id) || accountDisplay(id, '', t))
            : dimension === 'apikey' ? shortCaller(id) : id
        let name = base
        let suffix = 2
        while (usedNames.has(name)) name = `${base} (${suffix++})`
        usedNames.add(name)
        return {
            name,
            fullName: other ? '' : id,
            color: other ? '#94A3B8' : (PALETTES[dimension] || PALETTES.model)[index],
            valueOf: (day) => ids.reduce((sum, key) => sum + (day[field]?.[key] || 0), 0),
            valueOfCost: (day) => ids.reduce((sum, key) => sum + (day[costField]?.[key] || 0), 0),
        }
    })
}
