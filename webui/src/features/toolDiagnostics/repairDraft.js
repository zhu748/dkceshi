export function repairDraftBody(draft) {
    let extra
    try {
        extra = draft.extra?.trim() ? JSON.parse(draft.extra) : undefined
        if (extra !== undefined && (!extra || Array.isArray(extra) || typeof extra !== 'object' || new TextEncoder().encode(JSON.stringify(extra)).length > 8192)) throw new Error()
    } catch { throw new Error('invalidExtra') }
    const { extra: _extra, ...body } = draft
    return { ...body, extra_body: extra, retry_count: repairInteger(body.retry_count, 3, 0, 10, 'invalidRetries'), timeout_seconds: repairInteger(body.timeout_seconds, 10, 1, 300, 'invalidTimeout') }
}

export function canReuseRepairKey(settings, draft) {
 if (!settings?.has_api_key || draft.clear_api_key) return false
 if ((settings.protocol || 'openai') !== draft.protocol) return false
 try { return new URL(settings.base_url).origin === new URL(draft.base_url.trim()).origin }
 catch { return false }
}

export function repairThinkingModes(protocol) {
 return protocol === 'claude' ? ['auto','adaptive','budget'] : protocol === 'gemini' ? ['auto','level','budget'] : []
}

export function repairInteger(value, fallback, min, max, error) {
    if (value === undefined) return fallback
    if (typeof value !== 'number' || !Number.isInteger(value) || value < min || value > max) throw new Error(error)
    return value
}

export function repairDraftError(message, t) {
    return ['invalidExtra', 'invalidRetries', 'invalidTimeout'].includes(message) ? t(`toolDiagnostics.repair.${message}`) : message
}
