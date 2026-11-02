import { useEffect, useState } from 'react'
import { Settings2, Save, RefreshCcw } from 'lucide-react'
import { useRepairModels } from './useRepairModels'

const input = 'w-full rounded-lg border border-border bg-background px-3 py-2 text-sm disabled:opacity-50'

function toDraft(settings) {
    return {
        mode: settings?.mode || 'original',
        protocol: settings?.protocol || 'openai',
        base_url: settings?.base_url || '',
        model: settings?.model || '',
        api_key: '',
        clear_api_key: false,
        timeout_seconds: settings?.timeout_seconds || 10,
        extra: settings?.extra_body ? JSON.stringify(settings.extra_body, null, 2) : '',
    }
}

export default function RepairSettingsPanel({ settings, ready, busy, mutate, authFetch, t }) {
    const [draft, setDraft] = useState(() => toDraft(settings))
    const [dirty, setDirty] = useState(false)
    const [error, setError] = useState('')
    const modelList = useRepairModels(authFetch)
    const signature = JSON.stringify(settings || {})
    useEffect(() => { if (!dirty) { modelList.invalidate(); setDraft(toDraft(settings)) } }, [signature, dirty])
    const change = patch => {
        if (['protocol', 'base_url', 'api_key', 'clear_api_key', 'mode'].some(key => Object.hasOwn(patch, key))) modelList.invalidate()
        setDraft(previous => ({ ...previous, ...patch })); setDirty(true); setError('')
    }
    const urlPlaceholder = draft.protocol === 'gemini' ? 'https://generativelanguage.googleapis.com/v1beta' : draft.protocol === 'claude' ? 'https://api.anthropic.com/v1' : 'https://api.example.com/v1'

    const save = async event => {
        event.preventDefault()
        let extra
        try {
            extra = draft.extra.trim() ? JSON.parse(draft.extra) : undefined
            if (extra !== undefined && (!extra || Array.isArray(extra) || typeof extra !== 'object')) throw new Error()
        } catch { setError(t('toolDiagnostics.repair.invalidExtra')); return }
        const { extra: _extra, ...body } = draft
        const saved = await mutate('/repair-settings', 'PUT', { ...body, extra_body: extra })
        if (saved?.settings?.repair) { setDraft(toDraft(saved.settings.repair)); setDirty(false) }
    }

    return <details className="rounded-xl border border-border bg-card/70 p-4">
        <summary className="cursor-pointer text-sm font-semibold"><Settings2 className="mr-2 inline h-4 w-4 text-primary" />{t('toolDiagnostics.repair.title')}<span className="ml-3 text-xs font-normal text-muted-foreground">{settings?.mode === 'custom' ? settings.model : t('toolDiagnostics.repair.original')}</span></summary>
        <form onSubmit={save} className="mt-4 space-y-4">
            <p className="text-xs leading-relaxed text-muted-foreground">{t('toolDiagnostics.repair.help')}</p>
            <div className="grid gap-4 sm:grid-cols-2">
                <label className="space-y-2 text-xs"><span>{t('toolDiagnostics.repair.mode')}</span><select className={input} value={draft.mode} disabled={busy || !ready} onChange={event => change({ mode: event.target.value })}><option value="original">{t('toolDiagnostics.repair.original')}</option><option value="custom">{t('toolDiagnostics.repair.custom')}</option></select></label>
                <label className="space-y-2 text-xs"><span>{t('toolDiagnostics.repair.timeout')}</span><input className={input} type="number" min="1" max="300" required value={draft.timeout_seconds} disabled={busy || !ready} onChange={event => change({ timeout_seconds: event.target.value === '' ? '' : Number(event.target.value) })} /></label>
            </div>
            {draft.mode === 'custom' && <div className="space-y-4 rounded-lg border border-border p-4">
                <label className="block space-y-2 text-xs"><span>{t('toolDiagnostics.repair.protocol')}</span><select className={input} value={draft.protocol} disabled={busy || !ready} onChange={event => change({ protocol: event.target.value, model: '', extra: '' })}>{['openai', 'claude', 'gemini'].map(protocol => <option key={protocol} value={protocol}>{t(`toolDiagnostics.repair.protocol_${protocol}`)}</option>)}</select></label>
                <label className="block space-y-2 text-xs"><span>{t('toolDiagnostics.repair.url')}</span><input className={input} type="url" required maxLength={2048} value={draft.base_url} disabled={busy || !ready} placeholder={urlPlaceholder} onChange={event => change({ base_url: event.target.value })} /><span className="block text-muted-foreground">{t(`toolDiagnostics.repair.urlHelp_${draft.protocol}`)}</span></label>
                <div className="grid gap-4 sm:grid-cols-2">
                    <label className="space-y-2 text-xs"><span>{t('toolDiagnostics.repair.model')}</span><input className={input} required maxLength={256} value={draft.model} disabled={busy || !ready} placeholder="your-repair-model" onChange={event => change({ model: event.target.value })} /></label>
                    <label className="space-y-2 text-xs"><span>{t('toolDiagnostics.repair.key')}{settings?.has_api_key && <span className="ml-2 text-primary">{t('toolDiagnostics.repair.keySet')}</span>}</span><input className={input} type="password" autoComplete="new-password" maxLength={8192} value={draft.api_key} disabled={busy || !ready} placeholder={settings?.has_api_key ? t('toolDiagnostics.repair.keepKey') : t('toolDiagnostics.repair.optionalKey')} onChange={event => change({ api_key: event.target.value, clear_api_key: false })} /></label>
                </div>
                <div className="space-y-3 rounded-lg bg-secondary/30 p-3">
                    <div className="flex flex-wrap items-center justify-between gap-3"><p className="text-xs text-muted-foreground">{t('toolDiagnostics.repair.fetchHelp')}</p><button type="button" disabled={busy || !ready || modelList.loading || !draft.base_url.trim()} onClick={() => modelList.load(draft)} className="inline-flex items-center gap-2 rounded-lg border border-border bg-background px-3 py-2 text-xs disabled:opacity-40"><RefreshCcw className={`h-3.5 w-3.5 ${modelList.loading ? 'animate-spin' : ''}`} />{t(modelList.loading ? 'toolDiagnostics.repair.fetching' : 'toolDiagnostics.repair.fetchModels')}</button></div>
                    {modelList.models.length > 0 && <label className="block space-y-2 text-xs"><span>{t('toolDiagnostics.repair.selectModel', { count: modelList.models.length })}</span><select className={input} value={draft.model} disabled={busy} onChange={event => change({ model: event.target.value })}><option value="">{t('toolDiagnostics.repair.chooseModel')}</option>{draft.model && !modelList.models.some(model => model.id === draft.model) && <option value={draft.model}>{draft.model}</option>}{modelList.models.map(model => <option key={model.id} value={model.id}>{model.name === model.id ? model.id : `${model.name} (${model.id})`}</option>)}</select></label>}
                    {modelList.fetched && modelList.models.length === 0 && <p className="text-xs text-muted-foreground">{t('toolDiagnostics.repair.noModels')}</p>}
                    {modelList.truncated && <p className="text-xs text-amber-500">{t('toolDiagnostics.repair.modelsTruncated')}</p>}
                    {modelList.error && <p role="alert" className="break-words text-xs text-destructive">{modelList.error}</p>}
                </div>
                <p className="text-xs text-muted-foreground">{t('toolDiagnostics.repair.keyHelp')}</p>
                {settings?.has_api_key && <label className="flex items-center gap-2 text-xs"><input type="checkbox" checked={draft.clear_api_key} disabled={busy} onChange={event => change({ clear_api_key: event.target.checked, api_key: '' })} />{t('toolDiagnostics.repair.clearKey')}</label>}
                <details className="text-xs"><summary className="cursor-pointer">{t('toolDiagnostics.repair.extra')}</summary><textarea className={`${input} mt-2 min-h-28 font-mono text-xs`} maxLength={8192} value={draft.extra} disabled={busy || !ready} placeholder={draft.protocol === 'claude' ? '{"thinking":{"type":"adaptive"}}' : draft.protocol === 'gemini' ? '{"generationConfig":{"thinkingConfig":{"includeThoughts":true}}}' : '{"reasoning_effort":"high"}'} onChange={event => change({ extra: event.target.value })} /><p className="mt-2 leading-relaxed text-muted-foreground">{t('toolDiagnostics.repair.extraHelp')}</p></details>
            </div>}
            {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
            <div className="flex items-center justify-between gap-4"><p className="text-xs text-muted-foreground">{t('toolDiagnostics.repair.independent')}</p><button type="submit" disabled={busy || !ready || !dirty || modelList.loading} className="inline-flex shrink-0 items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground disabled:opacity-40"><Save className="h-4 w-4" />{t('toolDiagnostics.repair.save')}</button></div>
        </form>
    </details>
}
