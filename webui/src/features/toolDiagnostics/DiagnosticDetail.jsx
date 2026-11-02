import { Download, LockKeyhole, Trash2, X } from 'lucide-react'
import { useRef } from 'react'
import { useDialogA11y } from '../usage/useDialogA11y'

export function label(t, group, value) {
    if (!value) return '—'
    const key = `toolDiagnostics.${group}.${value}`
    const text = t(key)
    return text === key ? value : text
}

function Snapshot({ title, value, t }) {
    if (!value?.text) return null
    return <details className="rounded-lg border border-border bg-background/60 p-3">
        <summary className="cursor-pointer text-xs font-medium">{title} <span className="text-muted-foreground">({value.bytes?.toLocaleString()} B)</span></summary>
        {value.truncated && <p className="mt-2 text-xs text-amber-500">{t('toolDiagnostics.truncated')}</p>}
        <pre className="mt-3 max-h-80 overflow-auto whitespace-pre-wrap break-all text-xs leading-relaxed">{value.text}</pre>
    </details>
}

function downloadRecord(record) {
    const url = URL.createObjectURL(new Blob([JSON.stringify(record, null, 2)], { type: 'application/json' }))
    const link = document.createElement('a')
    link.href = url
    link.download = `${record.id}.json`
    link.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export default function DiagnosticDetail({ record, t, onDelete, busy }) {
    if (!record) return <div className="p-8 text-center text-sm text-muted-foreground">{t('toolDiagnostics.selectRecord')}</div>
    return <div className="space-y-5 p-4 sm:p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
                <div className="flex items-center gap-2 font-semibold">{label(t, 'status', record.status)}{record.pinned && <LockKeyhole className="h-4 w-4 text-amber-500" />}</div>
                <p className="mt-1 break-all font-mono text-[10px] text-muted-foreground">{record.id}</p>
                <p className="mt-2 text-xs text-muted-foreground">{record.surface} · {record.model} · {record.stream ? t('toolDiagnostics.streaming') : t('toolDiagnostics.nonStreaming')}</p>
                <p className="mt-1 text-xs">{t('toolDiagnostics.delivery')}: {label(t, 'result', record.delivery)}</p>
            </div>
            <div className="flex gap-1">
                <button type="button" className="rounded p-2 hover:bg-secondary" onClick={() => downloadRecord(record)} title={t('toolDiagnostics.download')}><Download className="h-4 w-4" /></button>
                <button type="button" disabled={busy || !record.completed_at} className="rounded p-2 text-destructive hover:bg-destructive/10 disabled:opacity-40" onClick={onDelete} title={t('toolDiagnostics.delete')}><Trash2 className="h-4 w-4" /></button>
            </div>
        </div>
        {record.pinned && <p className="rounded-lg border border-amber-500/25 bg-amber-500/5 p-3 text-xs text-amber-600 dark:text-amber-400">{t('toolDiagnostics.pinnedHelp')}</p>}
        {record.error && <div className="rounded-lg border border-destructive/25 bg-destructive/5 p-3 text-xs break-words"><span className="font-mono">{record.error_code}</span><p className="mt-1 whitespace-pre-wrap">{record.error}</p></div>}
        {record.attempts?.map(attempt => <section key={attempt.number} className="space-y-3 rounded-xl border border-border p-4">
            <div className="flex flex-wrap items-center justify-between gap-2 text-sm font-semibold">
                <span>{t('toolDiagnostics.attempt', { number: attempt.number })} · {label(t, 'kind', attempt.kind)}</span>
                <span className={attempt.result === 'failed' ? 'text-destructive' : 'text-primary'}>{label(t, 'result', attempt.result)}</span>
            </div>
            <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                <span>{label(t, 'method', attempt.method)}</span>
                {attempt.account_id && <span>{t('toolDiagnostics.account')}: {attempt.account_id}</span>}
                <span>{t('toolDiagnostics.source')}: {label(t, 'source', attempt.source)}</span>
                {attempt.completed_at && <span>{Math.max(0, attempt.completed_at - attempt.started_at).toLocaleString()} ms</span>}
            </div>
            {attempt.error && <p className="whitespace-pre-wrap break-words text-xs text-destructive">{attempt.error_code}: {attempt.error}</p>}
            {attempt.retry_decision && <p className="rounded bg-secondary/70 p-2 text-xs">{t('toolDiagnostics.retry')}: {label(t, 'result', attempt.retry_decision)}{attempt.retry_reason && ` · ${attempt.retry_reason}`}</p>}
            <div className="space-y-2">
                {attempt.calls?.map((call, index) => <div key={index} className="rounded-lg border border-border bg-secondary/25 p-3 text-xs">
                    <div className="flex flex-wrap justify-between gap-2"><strong className="font-mono">#{call.ordinal || '?'} {call.name || t('toolDiagnostics.unknownTool')}</strong><span>{label(t, 'result', call.delivery)}</span></div>
                    <div className="mt-2 flex flex-wrap gap-4"><span>{t('toolDiagnostics.parse')}: {label(t, 'result', call.parse)}</span><span>Schema: {label(t, 'result', call.schema)}</span></div>
                    {call.error && <p className="mt-2 whitespace-pre-wrap break-words text-destructive">{call.error}</p>}
                    <div className="mt-2"><Snapshot title={t('toolDiagnostics.arguments')} value={call.arguments} t={t} /></div>
                </div>)}
            </div>
            <details open={attempt.result === 'failed'} className="text-xs">
                <summary className="cursor-pointer font-medium">{t('toolDiagnostics.timeline')}</summary>
                <ol className="mt-3 space-y-3 border-l border-border pl-4">
                    {attempt.steps?.map((step, index) => <li key={index} className="space-y-2">
                        <p className="flex flex-wrap gap-2"><strong>{label(t, 'stage', step.stage)}</strong><span>{label(t, 'result', step.result)}</span>{step.elapsed_ms > 0 && <span className="text-muted-foreground">{step.elapsed_ms} ms</span>}</p>
                        {step.model && <p className="font-mono text-muted-foreground">{label(t, 'provider', step.provider)} · {step.model}{step.protocol && ` · ${t(`toolDiagnostics.repair.protocol_${step.protocol}`)}`}</p>}
                        {step.reason && <p className="text-muted-foreground">{label(t, 'reason', step.reason)}</p>}
                        {step.error && <p className="whitespace-pre-wrap break-words text-destructive">{step.error}</p>}
                        <Snapshot title={t('toolDiagnostics.repairInput')} value={step.input} t={t} />
                        <Snapshot title={t('toolDiagnostics.repairOutput')} value={step.output} t={t} />
                    </li>)}
                </ol>
            </details>
            <Snapshot title={t('toolDiagnostics.raw')} value={attempt.raw} t={t} />
            <Snapshot title={t('toolDiagnostics.residual')} value={attempt.residual} t={t} />
        </section>)}
        <Snapshot title={t('toolDiagnostics.catalog')} value={record.tools} t={t} />
        <Snapshot title={t('toolDiagnostics.policy')} value={record.policy} t={t} />
        <p className="text-xs text-muted-foreground">{t('toolDiagnostics.executionHelp')}</p>
    </div>
}

export function DiagnosticConfirm({ title, message, t, busy, onConfirm, onClose }) {
    const panel = useRef(null)
    useDialogA11y(panel, busy ? () => {} : onClose)
    return <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true" aria-label={title}>
        <div ref={panel} className="w-full max-w-md space-y-4 rounded-xl border border-border bg-card p-6 shadow-xl">
            <div className="flex justify-between gap-3"><h2 className="font-semibold">{title}</h2><button type="button" onClick={onClose} disabled={busy} aria-label={t('toolDiagnostics.cancel')}><X className="h-4 w-4" /></button></div>
            <p className="text-sm text-muted-foreground">{message}</p>
            <div className="flex justify-end gap-3"><button type="button" disabled={busy} onClick={onClose} className="rounded-lg border border-border px-3 py-2 text-sm">{t('toolDiagnostics.cancel')}</button><button type="button" disabled={busy} onClick={onConfirm} className="rounded-lg bg-destructive px-3 py-2 text-sm text-destructive-foreground">{t('toolDiagnostics.confirm')}</button></div>
        </div>
    </div>
}
