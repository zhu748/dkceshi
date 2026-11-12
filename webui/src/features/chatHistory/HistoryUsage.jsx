import { historyUsage, formatTokenCount } from './historyUsage.js'

export function HistoryUsageLine({ item, t }) {
    const usage = historyUsage(item)
    if (usage.pending) return <div className="mt-2 text-[11px] text-muted-foreground">{t('chatHistory.usagePending')}</div>
    return (
        <div className="mt-2 space-y-1 text-[11px] text-muted-foreground tabular-nums">
            <div className="flex flex-wrap gap-x-3 gap-y-1">
                <span>{t('chatHistory.usageInput')} {formatTokenCount(usage.input)}</span>
                <span>{t('chatHistory.usageOutput')} {formatTokenCount(usage.output)}</span>
                <span>{t('chatHistory.usageTotal')} {formatTokenCount(usage.total)}</span>
            </div>
            <div className="flex flex-wrap gap-x-3 gap-y-1">
                <span>{t(`chatHistory.usageSources.${usage.source}`)}</span>
                {usage.complete === false && <span>{t('chatHistory.usagePartial')}</span>}
                {usage.accumulated !== null && <span title={t('chatHistory.usageUpstreamHelp')}>{t('chatHistory.usageUpstream')} {formatTokenCount(usage.accumulated)}</span>}
            </div>
        </div>
    )
}

export default function HistoryUsage({ item, t }) {
    const usage = historyUsage(item)
    const cards = [['Input', usage.input, usage.inputSource], ['Output', usage.output, usage.outputSource], ['Reasoning', usage.reasoning, usage.reasoningSource], ['Total', usage.total, usage.totalSource]]
    return (
        <section aria-label={t('chatHistory.usageTitle')} className="max-w-4xl mx-auto rounded-xl border border-border bg-background/70 p-4 space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-xs font-semibold text-muted-foreground">{t('chatHistory.usageTitle')}</span>
                <span className="text-xs text-muted-foreground">{usage.pending ? t('chatHistory.usagePending') : usage.complete === false ? t('chatHistory.usagePartial') : t(`chatHistory.usageSources.${usage.source}`)}</span>
            </div>
            <div className="grid grid-cols-2 xl:grid-cols-4 gap-3">
                {cards.map(([label, value, source]) => (
                    <div key={label} className="rounded-lg border border-border bg-card px-3 py-2">
                        <div className="text-[11px] text-muted-foreground">{t(`chatHistory.usage${label}`)}</div>
                        <div className="text-lg font-semibold text-foreground tabular-nums">{formatTokenCount(value)}</div>
                        {value !== null && <div className="text-[10px] text-muted-foreground">{t(`chatHistory.usageSources.${source}`)}</div>}
                    </div>
                ))}
            </div>
            <p className="text-[11px] text-muted-foreground">{t('chatHistory.usageReasoningHelp')}</p>
            {usage.messages.length > 0 && (
                <div className="space-y-2 border-t border-border pt-3">
                    <div className="text-xs font-medium text-foreground">{t('chatHistory.usageUpstreamTitle')}</div>
                    {usage.messages.map((message, index) => (
                        <div key={`${message.message_id || 'unknown'}-${index}`} className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground tabular-nums">
                            <span>{t('chatHistory.usageMessage')} {message.message_id || '—'}</span>
                            <span>{t('chatHistory.usageUpstream')} {formatTokenCount(message.accumulated_tokens)}</span>
                            {countFields(message, t)}
                        </div>
                    ))}
                    <p className="text-[11px] text-muted-foreground">{t('chatHistory.usageUpstreamHelp')}</p>
                    {usage.truncated && <p className="text-[11px] text-muted-foreground">{t('chatHistory.usageTruncated')}</p>}
                </div>
            )}
        </section>
    )
}

function countFields(message, t) {
    return [['input_tokens', 'usageInput'], ['output_tokens', 'usageOutput'], ['reasoning_tokens', 'usageReasoning'], ['total_tokens', 'usageTotal']]
        .filter(([key]) => Number.isSafeInteger(message[key]) && message[key] >= 0)
        .map(([key, label]) => <span key={key}>{t(`chatHistory.${label}`)} {formatTokenCount(message[key])}</span>)
}
