import { ChevronLeft, ChevronRight, LockKeyhole, RefreshCcw, ScanLine } from 'lucide-react'
import { useState } from 'react'
import clsx from 'clsx'
import { useI18n } from '../../i18n'
import DiagnosticDetail, { DiagnosticConfirm, label } from './DiagnosticDetail'
import RepairSettingsPanel from './RepairSettingsPanel'
import { useToolDiagnostics } from './useToolDiagnostics'

const STATUSES = ['direct_success', 'local_recovery_success', 'llm_repair_success', 'regenerated_success', 'failed', 'interrupted', 'running']
const control = 'rounded-lg border border-border bg-background px-3 py-2 text-xs disabled:opacity-40'

export default function ToolDiagnosticsContainer({ authFetch, onMessage }) {
    const { t, lang } = useI18n()
    const state = useToolDiagnostics(authFetch, onMessage)
    const { data, query, setQuery, selected, setSelected, detail, error, busy, refresh, mutate } = state
    const [confirmation, setConfirmation] = useState(null)
    const settings = data?.settings || { enabled: true, limit: 20 }
    const updateQuery = patch => setQuery(previous => ({ ...previous, ...patch, page: 1 }))
    const clear = scope => setConfirmation({ path: `?scope=${scope}`, title: t(scope === 'errors' ? 'toolDiagnostics.clearErrors' : 'toolDiagnostics.clearNormal'), message: t(scope === 'errors' ? 'toolDiagnostics.clearErrorsConfirm' : 'toolDiagnostics.clearNormalConfirm') })

    return <div className="space-y-5">
        <div className="grid gap-3 sm:grid-cols-3">
            <div className="rounded-xl border border-border bg-card/70 p-4"><p className="text-xs text-muted-foreground">{t('toolDiagnostics.ordinaryLimit')}</p><p className="mt-2 text-2xl font-bold">{settings.limit}</p></div>
            <div className="rounded-xl border border-border bg-card/70 p-4"><p className="flex items-center gap-2 text-xs text-muted-foreground"><LockKeyhole className="h-3.5 w-3.5" />{t('toolDiagnostics.retainedErrors')}</p><p className="mt-2 text-2xl font-bold text-amber-500">{data?.pinned_count || 0}</p></div>
            <div className="rounded-xl border border-border bg-card/70 p-4"><p className="text-xs text-muted-foreground">{t('toolDiagnostics.matched')}</p><p className="mt-2 text-2xl font-bold">{data?.total || 0}</p></div>
        </div>
        <div className="flex flex-wrap items-center gap-3 rounded-xl border border-border bg-card/70 p-4">
            <label className="flex items-center gap-2 text-xs"><input type="checkbox" checked={settings.enabled} disabled={busy || !data} onChange={event => mutate('/settings', 'PUT', { ...settings, enabled: event.target.checked })} />{t('toolDiagnostics.enabled')}</label>
            <label className="flex items-center gap-2 text-xs">{t('toolDiagnostics.retain')}<select className={control} value={settings.limit} disabled={busy || !data} onChange={event => mutate('/settings', 'PUT', { ...settings, limit: Number(event.target.value) })}><option value={20}>20</option><option value={50}>50</option></select></label>
            <span className="flex-1" />
            <button type="button" className={control} disabled={busy} onClick={refresh} title={t('toolDiagnostics.refresh')}><RefreshCcw className="h-4 w-4" /></button>
            <button type="button" className={control} disabled={busy || !data} onClick={() => clear('normal')}>{t('toolDiagnostics.clearNormal')}</button>
            <button type="button" className={`${control} text-destructive`} disabled={busy || !data?.pinned_count} onClick={() => clear('errors')}>{t('toolDiagnostics.clearErrors')}</button>
        </div>
        <RepairSettingsPanel settings={settings.repair} ready={Boolean(data)} busy={busy} mutate={mutate} authFetch={authFetch} t={t} />
        <p className="text-xs text-muted-foreground">{t('toolDiagnostics.retentionHelp')}{!settings.enabled && ` ${t('toolDiagnostics.disabledHelp')}`}</p>
        {(error || data?.storage_error) && <div role="alert" className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{error || data.storage_error}</div>}
        <div className="flex flex-wrap items-center gap-3">
            <input className={`${control} min-w-48 flex-1`} value={query.search} onChange={event => updateQuery({ search: event.target.value })} placeholder={t('toolDiagnostics.search')} aria-label={t('toolDiagnostics.search')} />
            <select className={control} value={query.status} onChange={event => updateQuery({ status: event.target.value })} aria-label={t('toolDiagnostics.filterStatus')}><option value="">{t('toolDiagnostics.allStatuses')}</option>{STATUSES.map(status => <option key={status} value={status}>{label(t, 'status', status)}</option>)}</select>
            <label className="flex items-center gap-2 text-xs"><input type="checkbox" checked={query.pinned} onChange={event => updateQuery({ pinned: event.target.checked })} />{t('toolDiagnostics.onlyErrors')}</label>
        </div>
        <div className="grid items-start gap-4 xl:grid-cols-[minmax(260px,0.8fr)_minmax(0,1.6fr)]">
            <div className="overflow-hidden rounded-xl border border-border bg-card/70">
                {!data && !error && <p className="p-8 text-center text-sm text-muted-foreground">{t('toolDiagnostics.loading')}</p>}
                {data?.items.length === 0 && <div className="space-y-3 p-8 text-center text-sm text-muted-foreground"><ScanLine className="mx-auto h-8 w-8 opacity-40" /><p>{t('toolDiagnostics.empty')}</p></div>}
                {data?.items.map(item => <button type="button" key={item.id} onClick={() => setSelected(item.id)} className={clsx('block w-full space-y-2 border-b border-border p-4 text-left transition-colors last:border-b-0 hover:bg-secondary/50', item.id === selected && 'bg-primary/5 ring-1 ring-inset ring-primary/30')}>
                    <div className="flex items-center justify-between gap-2"><strong className={clsx('text-xs', item.status === 'failed' ? 'text-destructive' : 'text-foreground')}>{label(t, 'status', item.status)}</strong>{item.pinned && <LockKeyhole className="h-3.5 w-3.5 shrink-0 text-amber-500" />}</div>
                    <p className="break-all font-mono text-xs">{item.tool_names?.filter(Boolean).join(' · ') || t('toolDiagnostics.unknownTool')}</p>
                    <p className="text-[11px] text-muted-foreground">{item.model} · {item.attempt_count} {t('toolDiagnostics.attempts')}</p>
                    <p className="text-[10px] text-muted-foreground">{new Date(item.created_at).toLocaleString(lang === 'zh' ? 'zh-CN' : 'en-US')}</p>
                </button>)}
                <div className="flex items-center justify-between gap-2 border-t border-border px-3 py-2 text-xs text-muted-foreground">
                    <button type="button" disabled={query.page === 1} className="rounded p-2 hover:bg-secondary disabled:opacity-30" aria-label={t('toolDiagnostics.previous')} onClick={() => setQuery(previous => ({ ...previous, page: previous.page - 1 }))}><ChevronLeft className="h-4 w-4" /></button>
                    <span>{query.page} / {Math.max(1, Math.ceil((data?.total || 0) / 20))}</span>
                    <button type="button" disabled={!data || query.page * 20 >= data.total} className="rounded p-2 hover:bg-secondary disabled:opacity-30" aria-label={t('toolDiagnostics.next')} onClick={() => setQuery(previous => ({ ...previous, page: previous.page + 1 }))}><ChevronRight className="h-4 w-4" /></button>
                </div>
            </div>
            <div className="min-w-0 rounded-xl border border-border bg-card/70"><DiagnosticDetail record={detail} t={t} busy={busy} authFetch={authFetch} onDelete={() => setConfirmation({ path: `/${encodeURIComponent(selected)}`, title: t('toolDiagnostics.delete'), message: t('toolDiagnostics.deleteConfirm') })} /></div>
        </div>
        {confirmation && <DiagnosticConfirm {...confirmation} t={t} busy={busy} onClose={() => setConfirmation(null)} onConfirm={async () => { await mutate(confirmation.path, 'DELETE'); setConfirmation(null) }} />}
    </div>
}
