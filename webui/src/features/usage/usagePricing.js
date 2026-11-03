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

export function toServerUsageSettings(nextPricing, nextEnabled) {
    return {
        enabled: !!nextEnabled,
        models: Object.fromEntries(
            Object.entries(nextPricing.models || {}).map(([name, p]) => [
                name,
                {
                    input_price: Number(p?.inputPrice || 0),
                    // 显式下发未命中单价：服务端把缺省字段当作「旧设置
                    // 文件」并回落默认值，只有写出来才能保存用户填的 0。
                    input_miss_price: Number(p?.inputMissPrice ?? DEFAULT_INPUT_MISS_PRICE),
                    output_price: Number(p?.outputPrice || 0),
                },
            ])
        ),
        peak: {
            enabled: !!nextPricing.peakEnabled,
            start1: nextPricing.peak1Start,
            end1: nextPricing.peak1End,
            start2: nextPricing.peak2Start,
            end2: nextPricing.peak2End,
            multiplier: Number(nextPricing.peakMultiplier || 0),
            weekend_normal: !!nextPricing.weekendNormal,
        },
        cache_hit: {
            enabled: !!nextPricing.cacheHitEnabled,
            rate: Number(nextPricing.cacheHitRate ?? DEFAULT_CACHE_HIT_RATE),
        },
    }
}
