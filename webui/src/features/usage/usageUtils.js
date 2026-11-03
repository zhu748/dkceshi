export const GMT8_OFFSET_MS = 8 * 60 * 60 * 1000

// parseDayKey turns a GMT+8 "YYYY-MM-DD" ledger key back into a Date at the
// start of that Beijing day. Centralised so every day-boundary calculation
// uses the exact same offset instead of scattering the literal around.
export function parseDayKey(key) {
    return new Date(key + 'T00:00:00+08:00')
}

export function fmtNumber(n, lang = 'zh') {
    return Math.round(Number(n || 0)).toLocaleString(lang === 'zh' ? 'zh-CN' : 'en-US')
}

// Keep one decimal for compact axis labels so 1.5K is not shown as "2K".
function trimCompact(value) {
    const rounded = Math.round(value * 10) / 10
    return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(1)
}

export function fmtCompact(n) {
    const value = Number(n || 0)
    const abs = Math.abs(value)
    if (abs >= 1e9) return trimCompact(value / 1e9) + 'B'
    if (abs >= 1e6) return trimCompact(value / 1e6) + 'M'
    if (abs >= 1e3) return trimCompact(value / 1e3) + 'K'
    return Math.round(value).toLocaleString('en-US')
}

export function fmtMoney(n, lang = 'zh', maxFractionDigits = 2) {
    const formatted = Number(n || 0).toLocaleString(lang === 'zh' ? 'zh-CN' : 'en-US', { minimumFractionDigits: 2, maximumFractionDigits: maxFractionDigits })
    return (lang === 'zh' ? '¥' : 'CN¥ ') + formatted
}

export function normalizeUsage(usage) {
    if (!usage) return null
    const prompt = Number(usage.prompt_tokens || 0)
    const completion = Number(usage.completion_tokens || 0)
    const reasoning = Number(usage.completion_tokens_details ? usage.completion_tokens_details.reasoning_tokens : 0) || 0
    const total = Number(usage.total_tokens || 0) || (prompt + completion)
    return { prompt, completion, reasoning, total }
}

export function dayKey(ts) {
    const shifted = new Date(Number(ts) + GMT8_OFFSET_MS)
    if (Number.isNaN(shifted.getTime())) return 'unknown'
    const y = shifted.getUTCFullYear()
    const m = String(shifted.getUTCMonth() + 1).padStart(2, '0')
    const day = String(shifted.getUTCDate()).padStart(2, '0')
    return y + '-' + m + '-' + day
}

export function dayLabel(key) {
    if (!key || key === 'unknown') return key || ''
    const parts = key.split('-')
    return parts[1] + '/' + parts[2]
}

export function shortCaller(caller) {
    const value = String(caller || 'unknown')
    if (value.startsWith('caller:')) return value.slice(7, 13)
    return value.slice(0, 8)
}

// Imported timestamps must remain representable by Intl.DateTimeFormat.
export function validUsageTimestamp(value) {
    const n = Number(value || 0)
    return Number.isSafeInteger(n) && n > 0 && n <= 8640000000000000 ? n : 0
}

export function entryToRecord(entry) {
    const date = String(entry.date || '')
    const usage = {
        prompt_tokens: Number(entry.prompt_tokens || 0),
        completion_tokens: Number(entry.completion_tokens || 0),
        total_tokens: Number(entry.total_tokens || 0),
        completion_tokens_details: { reasoning_tokens: Number(entry.reasoning_tokens || 0) },
    }
    return {
        model: entry.model || 'unknown',
        caller_id: entry.caller_id || '',
        account_id: entry.account_id || 'unknown',
        account_label: entry.account_label || '',
        last_used_at: validUsageTimestamp(entry.last_used_at),
        date,
        created_at: date ? parseDayKey(date).getTime() : 0,
        calls_count: Number(entry.calls || 0),
        usage,
        cost: Number(entry.cost || 0),
    }
}

// sameUsageRecords reports whether two mapped record lists carry the same
// values. The 30s poll rebuilds the list on every tick even when the ledger
// did not change; comparing lets the hook keep the previous array reference so
// downstream memos and charts are not invalidated for nothing.
export function sameUsageRecords(a, b) {
    if (a === b) return true
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false
    for (let i = 0; i < a.length; i++) {
        const x = a[i]
        const y = b[i]
        if (x.model !== y.model || x.caller_id !== y.caller_id || x.date !== y.date ||
            x.created_at !== y.created_at || x.calls_count !== y.calls_count || x.cost !== y.cost ||
            x.account_id !== y.account_id || x.account_label !== y.account_label || x.last_used_at !== y.last_used_at) {
            return false
        }
        const xu = x.usage
        const yu = y.usage
        if (!xu || !yu) {
            if (xu !== yu) return false
            continue
        }
        if (xu.prompt_tokens !== yu.prompt_tokens || xu.completion_tokens !== yu.completion_tokens ||
            xu.total_tokens !== yu.total_tokens ||
            xu.completion_tokens_details?.reasoning_tokens !== yu.completion_tokens_details?.reasoning_tokens) {
            return false
        }
    }
    return true
}
