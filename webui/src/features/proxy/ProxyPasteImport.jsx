import { EMPTY_PROXY_FORM, proxyEndpoint } from './proxyFields'
import { useEffect, useRef, useState } from 'react'

export default function ProxyPasteImport({t, apiFetch, form, setForm, isEditing, onImported, onBlocksSave}) {
    const [text, setText] = useState('')
    const [rows, setRows] = useState([])
    const [error, setError] = useState('')
    const [busy, setBusy] = useState(false)
    const [parsing, setParsing] = useState(false)
    const generation = useRef(0)
    const appliedType = useRef(null)
    const alive = useRef(true)
    const importing = useRef(false)
    const importController = useRef(null)
    const translate = useRef(t)
    translate.current = t
    useEffect(() => {
        alive.current = true
        setBusy(false)
        return () => {
            alive.current = false
            generation.current++
            importController.current?.abort()
            importController.current = null
            importing.current = false
        }
    }, [apiFetch])
    const ready = rows.filter(row => row.status === 'ready')
    useEffect(() => {
        onBlocksSave(parsing || busy || rows.length > 1 || rows.some(row => row.status === 'invalid') || (rows.length === 1 && !['ready', 'duplicate', 'incomplete'].includes(rows[0].status)) || Boolean(error))
        return () => onBlocksSave(false)
    }, [parsing, busy, rows, error, onBlocksSave])
    useEffect(() => {
        const applied = appliedType.current
        appliedType.current = null
        if (applied?.text === text && applied.type === form.type && applied.apiFetch === apiFetch) return
        const token = ++generation.current
        const controller = new AbortController()
        setRows([])
        setError('')
        setParsing(Boolean(text.trim()))
        if (!text.trim()) return () => controller.abort()
        const timer = setTimeout(async () => {
            try {
                const res = await apiFetch('/admin/proxies/parse', {method: 'POST', signal: controller.signal, headers: {'Content-Type': 'application/json'}, body: JSON.stringify({text, type: form.type})})
                const data = await res.json()
                if (generation.current !== token) return
                if (!res.ok) throw new Error(data.detail || translate.current('messages.requestFailed'))
                const items = data.items || []
                setRows(items)
                if (items.length === 1 && ['ready', 'duplicate', 'incomplete'].includes(items[0].status) && items[0].proxy) {
                    const proxy = items[0].proxy
                    if (proxy.type !== form.type) appliedType.current = {text, type: proxy.type, apiFetch}
                    setForm({...EMPTY_PROXY_FORM, ...proxy, password: proxy.password || '', clear_password: false})
                }
            } catch (err) {
                if (generation.current === token && err.name !== 'AbortError') setError(err.message || translate.current('messages.networkError'))
            } finally {
                if (generation.current === token) setParsing(false)
            }
        }, 350)
        return () => {clearTimeout(timer); generation.current++; controller.abort()}
    }, [text, form.type, apiFetch, setForm])

    const importReady = async () => {
        if (importing.current || busy || parsing || !ready.length) return
        importing.current = true
        const controller = new AbortController()
        importController.current = controller
        setBusy(true)
        setError('')
        try {
            const res = await apiFetch('/admin/proxies/import', {method: 'POST', signal: controller.signal, headers: {'Content-Type': 'application/json'}, body: JSON.stringify({items: ready.map(row => row.proxy)})})
            const data = await res.json()
            if (!res.ok) throw new Error(data.detail || translate.current('messages.requestFailed'))
            if (!alive.current || importController.current !== controller) return
            setText('')
            await onImported(data)
        } catch (err) {if (alive.current && importController.current === controller && err.name !== 'AbortError') setError(err.message || translate.current('messages.networkError'))}
        finally {if (importController.current === controller) {importing.current = false; importController.current = null; if (alive.current) setBusy(false)}}
    }
    const updateText = value => {
        appliedType.current = null
        generation.current++
        setRows([])
        setError('')
        setParsing(Boolean(value.trim()))
        onBlocksSave(Boolean(value.trim()))
        setText(value)
    }
    return <div className="rounded-lg border border-border bg-muted/10 p-3 space-y-2">
        <label className="block text-sm font-medium">{t('proxyManager.pasteLabel')}</label>
        <p className="text-xs text-muted-foreground">{t('proxyManager.pasteHint')}</p>
        <textarea className="input-field font-mono text-xs min-h-24" value={text} disabled={busy} onChange={e => updateText(e.target.value)} placeholder="host:1080:user:password" autoComplete="off" spellCheck={false} />
        {parsing && <p className="text-xs text-muted-foreground">{t('proxyManager.parsing')}</p>}
        {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
        {rows.length > 0 && <div className="max-h-48 overflow-y-auto space-y-1">
            {rows.map((row, index) => <div key={index} className="text-xs rounded border border-border p-2">
                <span className={row.status === 'invalid' ? 'text-destructive' : 'text-muted-foreground'}>{row.source} · {t(`proxyManager.importStatus.${row.status}`)}</span>
                {row.proxy && <span className="ml-2 break-all">{row.proxy.type.toUpperCase()} · {proxyEndpoint(row.proxy)}</span>}
                {row.message && <p className="mt-1 text-muted-foreground">{row.message}</p>}
            </div>)}
        </div>}
        {rows.length > 1 && (isEditing
            ? <p className="text-xs text-amber-500">{t('proxyManager.singleEditOnly')}</p>
            : <button disabled={busy || parsing || !ready.length} onClick={importReady} className="px-3 py-2 rounded-lg bg-primary text-primary-foreground text-sm disabled:opacity-50">{busy ? t('proxyManager.saving') : t('proxyManager.importReady', {count: ready.length})}</button>)}
    </div>
}
