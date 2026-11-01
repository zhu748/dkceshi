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

export function aggregateDaily(records) {
    const map = new Map()
    records.forEach((r) => {
        const usage = normalizeUsage(r.usage)
        if (!usage) return
        const key = dayKey(r.created_at || r.createdAt)
        const ts = parseDayKey(key).getTime()
        const entry = map.get(key) || {
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

        map.set(key, entry)
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

export function applyServerSettings(prev, s) {
    // 单价表固定只有一条共享条目，直接按 canonical 键重建，避免历史缓存里的
    // 旧档位键（pro / vision / flash / v4-pro）残留成多余输入框。
    const models = {
        [MODEL_PRICE_KEY]: prev.models?.[MODEL_PRICE_KEY] || {
            inputPrice: 0,
            inputMissPrice: DEFAULT_INPUT_MISS_PRICE,
            outputPrice: 0,
        },
    }
    const server = collapseModelPrices(s?.models)[MODEL_PRICE_KEY]
    if (server) {
        const fallback = models[MODEL_PRICE_KEY]
        const rawInput = server.input_price
        const rawOutput = server.output_price
        // 未命中单价是本次新增字段：服务端为 null/undefined（旧设置文件）时
        // 必须回落到默认值 1，而不是被 Number(null)=0 覆盖成免费。
        const rawMiss = server.input_miss_price
        models[MODEL_PRICE_KEY] = {
            inputPrice: Number.isFinite(Number(rawInput)) && Number(rawInput) >= 0 ? Number(rawInput) : fallback.inputPrice,
            inputMissPrice: rawMiss == null
                ? fallback.inputMissPrice
                : (Number.isFinite(Number(rawMiss)) && Number(rawMiss) >= 0 ? Number(rawMiss) : fallback.inputMissPrice),
            outputPrice: Number.isFinite(Number(rawOutput)) && Number(rawOutput) >= 0 ? Number(rawOutput) : fallback.outputPrice,
        }
    }
    const rawMultiplier = s.peak?.multiplier ?? prev.peakMultiplier
    const peakMultiplier = Number.isFinite(Number(rawMultiplier)) && Number(rawMultiplier) >= 0 ? Number(rawMultiplier) : prev.peakMultiplier
    const rawCacheHitRate = s.cache_hit?.rate
    const cacheHitRate = Number.isFinite(Number(rawCacheHitRate)) && Number(rawCacheHitRate) >= 0
        ? clampCacheHitRate(Number(rawCacheHitRate))
        : prev.cacheHitRate
    return {
        ...prev,
        models,
        peakEnabled: s.peak?.enabled ?? prev.peakEnabled,
        peak1Start: s.peak?.start1 ?? prev.peak1Start,
        peak1End: s.peak?.end1 ?? prev.peak1End,
        peak2Start: s.peak?.start2 ?? prev.peak2Start,
        peak2End: s.peak?.end2 ?? prev.peak2End,
        peakMultiplier,
        weekendNormal: s.peak?.weekend_normal ?? prev.weekendNormal,
        cacheHitEnabled: s.cache_hit?.enabled ?? prev.cacheHitEnabled,
        cacheHitRate,
    }
}

const PRICING_STORAGE_KEY = 'ds2api_usage_pricing_v2'

// 模型合并后所有公开模型（deepseek-v4.1-flash 及其 -search / -nothinking 变体）
// 共用同一套单价，因此单价表只保留这一条；与后端 internal/usagestats 的
// mergedModelPriceKey 保持一致。
export const MODEL_PRICE_KEY = 'deepseek-v4.1-flash'

// 「模拟缓存命中」的默认值：默认开启，命中率 98%，缓存未命中的输入单价 1 元。
// 与后端 internal/usagestats 的 defaultCacheHitRate / defaultInputMissPrice 保持一致。
export const DEFAULT_CACHE_HIT_RATE = 98
export const DEFAULT_INPUT_MISS_PRICE = 1

export function clampCacheHitRate(rate) {
    const n = Number(rate)
    if (!Number.isFinite(n)) return DEFAULT_CACHE_HIT_RATE
    if (n < 0) return 0
    if (n > 100) return 100
    return n
}

// splitCacheHitTokens 按「模拟缓存命中」配置把输入 tokens 拆成命中 / 未命中
// 两部分：命中部分 = 输入 tokens × 命中率，其余算未命中。上游不返回缓存命中
// 信息，这里只是按设置里的命中率模拟，口径与后端 inputCost 的计价拆分一致。
export function splitCacheHitTokens(prompt, rate) {
    const total = Math.max(0, Math.round(Number(prompt) || 0))
    const hit = Math.round(total * clampCacheHitRate(rate) / 100)
    return { hit, miss: total - hit }
}

// peakWindowError validates the two peak windows when peak billing is on.
// Times are zero-padded "HH:MM" strings so lexicographic comparison matches
// wall-clock order; the windows are order-independent and must not overlap.
// Returns an i18n key describing the problem, or null when valid.
export function peakWindowError(pricing) {
    if (!pricing?.peakEnabled) return null
    const { peak1Start, peak1End, peak2Start, peak2End } = pricing
    if (peak1Start >= peak1End || peak2Start >= peak2End) return 'usage.settingsPeakOrder'
    if (peak1Start < peak2End && peak2Start < peak1End) return 'usage.settingsPeakOverlap'
    return null
}

// 收敛单价表时继承既有取值的顺序，与后端 modelPriceInheritOrder 保持一致：
// 合并后的基础键优先，其次是合并后一度单列的 search 键，最后才是合并前的档位键。
const MODEL_PRICE_INHERIT_ORDER = [
    MODEL_PRICE_KEY,
    'deepseek-v4.1-flash-search',
    'flash',
    'vision',
    'pro',
    'v4-pro',
]

// collapseModelPrices reduces the price table to the single shared entry so the
// settings page only ever renders one input/output pair. The first key present
// in MODEL_PRICE_INHERIT_ORDER wins; every other key (pre-merge tiers, the
// former standalone search key, hand-edited leftovers) is dropped.
function collapseModelPrices(models) {
    if (!models || typeof models !== 'object') {
        return {}
    }
    for (const key of MODEL_PRICE_INHERIT_ORDER) {
        if (Object.prototype.hasOwnProperty.call(models, key)) {
            return { [MODEL_PRICE_KEY]: models[key] }
        }
    }
    return {}
}

export function defaultPricing() {
    return {
        models: {
            [MODEL_PRICE_KEY]: { inputPrice: 0.02, inputMissPrice: DEFAULT_INPUT_MISS_PRICE, outputPrice: 4 },
        },
        peakEnabled: true,
        peak1Start: '09:00',
        peak1End: '12:00',
        peak2Start: '14:00',
        peak2End: '18:00',
        peakMultiplier: 2,
        weekendNormal: true,
        cacheHitEnabled: true,
        cacheHitRate: DEFAULT_CACHE_HIT_RATE,
    }
}

export function loadLocalPricing() {
    const defaults = defaultPricing()
    try {
        const raw = localStorage.getItem(PRICING_STORAGE_KEY)
        if (!raw) return defaults
        const parsed = JSON.parse(raw)
        const stored = collapseModelPrices(parsed.models)[MODEL_PRICE_KEY]
        const fallback = defaults.models[MODEL_PRICE_KEY]
        const rawInput = stored?.inputPrice ?? stored?.input_price
        const rawOutput = stored?.outputPrice ?? stored?.output_price
        // 本地缓存里的旧记录没有未命中单价，必须回落到默认值而不是当成 0。
        const rawMiss = stored?.inputMissPrice ?? stored?.input_miss_price
        const models = {
            [MODEL_PRICE_KEY]: {
                inputPrice: Number.isFinite(Number(rawInput)) && Number(rawInput) >= 0 ? Number(rawInput) : fallback.inputPrice,
                inputMissPrice: rawMiss == null
                    ? fallback.inputMissPrice
                    : (Number.isFinite(Number(rawMiss)) && Number(rawMiss) >= 0 ? Number(rawMiss) : fallback.inputMissPrice),
                outputPrice: Number.isFinite(Number(rawOutput)) && Number(rawOutput) >= 0 ? Number(rawOutput) : fallback.outputPrice,
            },
        }
        return {
            ...defaults,
            ...parsed,
            models,
            peakEnabled: typeof parsed.peakEnabled === 'boolean' ? parsed.peakEnabled : defaults.peakEnabled,
            weekendNormal: typeof parsed.weekendNormal === 'boolean' ? parsed.weekendNormal : defaults.weekendNormal,
            cacheHitEnabled: typeof parsed.cacheHitEnabled === 'boolean' ? parsed.cacheHitEnabled : defaults.cacheHitEnabled,
            cacheHitRate: Number.isFinite(Number(parsed.cacheHitRate)) && Number(parsed.cacheHitRate) >= 0
                ? clampCacheHitRate(Number(parsed.cacheHitRate))
                : defaults.cacheHitRate,
            peakMultiplier: Number.isFinite(Number(parsed.peakMultiplier)) && Number(parsed.peakMultiplier) >= 0
                ? Number(parsed.peakMultiplier)
                : defaults.peakMultiplier,
            peak1Start: parsed.peak1Start ?? defaults.peak1Start,
            peak1End: parsed.peak1End ?? defaults.peak1End,
            peak2Start: parsed.peak2Start ?? defaults.peak2Start,
            peak2End: parsed.peak2End ?? defaults.peak2End,
        }
    } catch (e) {
        return defaults
    }
}

export function persistLocalPricing(pricing) {
    try {
        localStorage.setItem(PRICING_STORAGE_KEY, JSON.stringify(pricing))
    } catch (e) {
        // ignore invalid localStorage values
    }
}
