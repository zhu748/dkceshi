export function repairDraftBody(draft) {
    let extra
    try {
        extra = draft.extra?.trim() ? JSON.parse(draft.extra) : undefined
        if (extra !== undefined && (!extra || Array.isArray(extra) || typeof extra !== 'object' || new TextEncoder().encode(JSON.stringify(extra)).length > 8192)) throw new Error()
    } catch { throw new Error('invalidExtra') }
    const { extra: _extra, ...body } = draft
    return { ...body, extra_body: extra }
}
