const count = value => Number.isSafeInteger(value) && value >= 0 ? value : null
const first = (...values) => values.map(count).find(value => value !== null) ?? null

export function historyUsage(item) {
    const usage = item?.usage || {}
    const metadata = usage.ds2api || {}
    const messages = Array.isArray(metadata.upstream?.messages) ? metadata.upstream.messages : []
    const sources = ['upstream', 'estimated', 'mixed', 'derived', 'unknown']
    const source = value => sources.includes(value) ? value : 'unknown'
    return {
        input: first(usage.prompt_tokens, usage.input_tokens),
        output: first(usage.completion_tokens, usage.output_tokens),
        reasoning: metadata.reasoning_source === 'unknown' ? null : first(usage.reasoning_tokens, usage.completion_tokens_details?.reasoning_tokens, usage.output_tokens_details?.reasoning_tokens),
        total: first(usage.total_tokens),
        source: source(usage.source || metadata.source),
        inputSource: source(metadata.input_source || usage.source),
        outputSource: source(metadata.output_source || usage.source),
        reasoningSource: source(metadata.reasoning_source || usage.source),
        totalSource: source(metadata.total_source || usage.source),
        complete: typeof metadata.complete === 'boolean' ? metadata.complete : usage.complete,
        accumulated: first(usage.upstream_accumulated_tokens, messages.at(-1)?.accumulated_tokens),
        messages,
        truncated: metadata.upstream?.truncated === true,
        pending: item?.status === 'streaming' && !item?.usage,
    }
}

export function formatTokenCount(value) {
    return count(value) === null ? '—' : value.toLocaleString()
}
