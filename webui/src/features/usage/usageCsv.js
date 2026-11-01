import { dayKey, normalizeUsage, parseDayKey } from './usageUtils.js'

export function csvEscape(value) {
    const s = value == null ? '' : String(value)
    if (/[",\r\n]/.test(s)) return '"' + s.replace(/"/g, '""') + '"'
    return s
}

// Spreadsheet programs can evaluate formula-looking text even inside quoted
// CSV cells. Escape it reversibly; the marker column keeps legacy apostrophes
// intact when importing files that were not produced by this exporter.
function safeText(value) {
    const text = String(value || '')
    return text.startsWith("'") || /^[\s\u0000-\u001f]*[=+\-@]/u.test(text) ? "'" + text : text
}

// buildUsageCsv renders aggregated usage entries as CSV. Dates are the GMT+8
// day keys straight from the ledger (not re-encoded through toISOString,
// which used to shift every row to the previous UTC day).
export function buildUsageCsv(records) {
    const header = ['model', 'caller_id', 'date', 'prompt_tokens', 'completion_tokens', 'reasoning_tokens', 'total_tokens', 'calls', 'cost', 'account_id', 'account_label', 'last_used_at', 'text_encoding']
    const rows = []
    for (const r of records) {
        const u = normalizeUsage(r.usage)
        if (!u) continue
        rows.push([
            safeText(r.model || 'unknown'),
            safeText(r.caller_id),
            r.date || '',
            u.prompt,
            u.completion,
            u.reasoning,
            u.total,
            r.calls_count || 0,
            Number(r.cost || 0),
            safeText(r.account_id),
            safeText(r.account_label),
            r.last_used_at || '',
            'apostrophe-v1',
        ].map(csvEscape).join(','))
    }
    const body = rows.length ? rows.join('\n') + '\n' : ''
    return { csv: '\uFEFF' + header.join(',') + '\n' + body, count: rows.length }
}

// parseCsvRows 是一个最小但符合 RFC 4180 的 CSV 读取器：支持引号包裹、字段内
// 的逗号/换行与 "" 转义，并吃掉导出的 UTF-8 BOM。只服务于导入「导出的 CSV」，
// 不追求支持所有方言。
function parseCsvRows(text) {
    const rows = []
    let row = [], field = '', inQuotes = false, closedQuote = false
    const src = String(text || '').replace(/^\uFEFF/, '')
    const endField = () => { row.push(field); field = ''; closedQuote = false }
    for (let i = 0; i < src.length; i++) {
        const ch = src[i]
        if (inQuotes) {
            if (ch === '"') {
                if (src[i + 1] === '"') { field += '"'; i++ }
                else { inQuotes = false; closedQuote = true }
            } else field += ch
        } else if (ch === ',') endField()
        else if (ch === '\n') { endField(); rows.push(row); row = [] }
        else if (ch === '\r') continue
        else if (closedQuote) {
            if (ch !== ' ' && ch !== '\t') return null
        } else if (ch === '"') {
            if (field !== '') return null
            inQuotes = true
        } else field += ch
    }
    if (inQuotes) return null
    if (field !== '' || row.length || closedQuote) { endField(); rows.push(row) }
    return rows
}

function numericCell(value, integer = true) {
    const n = Number(String(value == null ? '' : value).replace(/,/g, '').trim())
    return Number.isFinite(n) && n >= 0 && (!integer || Number.isSafeInteger(n)) ? n : null
}

// parseUsageCsv 解析「导出」按钮产出的 CSV，得到可直接提交给
// /admin/usage/import 的条目数组。无法识别的表头/空文件返回 i18n key 形式的
// error；个别坏行（日期或模型缺失）会被跳过并通过 skipped 反馈。
export function parseUsageCsv(text) {
    const parsed = parseCsvRows(text)
    if (!parsed) return { entries: [], skipped: 0, error: 'usage.importInvalidCsv' }
    const rows = parsed.filter((r) => r.some((c) => String(c).trim() !== ''))
    if (rows.length === 0) {
        return { entries: [], skipped: 0, error: 'usage.importEmpty' }
    }
    const header = rows[0].map((h) => String(h).trim().toLowerCase())
    const col = (name) => header.indexOf(name)
    const columns = {
        model: col('model'),
        caller_id: col('caller_id'),
        date: col('date'),
        prompt_tokens: col('prompt_tokens'),
        completion_tokens: col('completion_tokens'),
        reasoning_tokens: col('reasoning_tokens'),
        total_tokens: col('total_tokens'),
        calls: col('calls'),
        cost: col('cost'),
        account_id: col('account_id'),
        account_label: col('account_label'),
        last_used_at: col('last_used_at'),
        text_encoding: col('text_encoding'),
    }
    if (columns.model === -1 || columns.date === -1 || columns.prompt_tokens === -1 || columns.completion_tokens === -1) {
        return { entries: [], skipped: 0, error: 'usage.importInvalidHeader' }
    }
    const entries = []
    let skipped = 0
    for (const row of rows.slice(1)) {
        const rawCell = (index) => (index >= 0 && index < row.length ? String(row[index]).trim() : '')
        const encoded = rawCell(columns.text_encoding) === 'apostrophe-v1'
        const textColumns = [columns.model, columns.caller_id, columns.account_id, columns.account_label]
        const cell = (index) => {
            const value = rawCell(index)
            return encoded && textColumns.includes(index) && value.startsWith("'") ? value.slice(1) : value
        }
        const date = cell(columns.date)
        const model = cell(columns.model)
        if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || dayKey(parseDayKey(date).getTime()) !== date || !model || [model, cell(columns.caller_id), cell(columns.account_id)].some((id) => id.includes('\0'))) {
            skipped++
            continue
        }
        const prompt = numericCell(cell(columns.prompt_tokens))
        const completion = numericCell(cell(columns.completion_tokens))
        const reasoning = numericCell(cell(columns.reasoning_tokens))
        const totalRaw = numericCell(cell(columns.total_tokens))
        const calls = numericCell(cell(columns.calls))
        const cost = numericCell(cell(columns.cost), false)
        const lastUsedAt = numericCell(cell(columns.last_used_at))
        const total = totalRaw === 0 ? prompt + completion : totalRaw
        if ([prompt, completion, reasoning, totalRaw, calls, cost, lastUsedAt].includes(null) ||
            !Number.isSafeInteger(total) || total < prompt + completion || reasoning > completion || lastUsedAt > 8640000000000000) {
            skipped++
            continue
        }
        entries.push({
            date,
            model,
            caller_id: cell(columns.caller_id),
            prompt_tokens: prompt,
            completion_tokens: completion,
            reasoning_tokens: reasoning,
            total_tokens: total,
            calls,
            cost,
            ...(cell(columns.account_id) ? { account_id: cell(columns.account_id) } : {}),
            ...(cell(columns.account_label) ? { account_label: cell(columns.account_label) } : {}),
            ...(cell(columns.last_used_at) ? { last_used_at: lastUsedAt } : {}),
        })
    }
    if (entries.length === 0) {
        return { entries: [], skipped, error: 'usage.importNoRows' }
    }
    return { entries, skipped, error: null }
}
