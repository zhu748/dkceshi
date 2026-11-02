import { useEffect, useState } from 'react'
import { readDiagnosticResponse } from './diagnosticRequests'

// Read a linked archive on demand; expired/cleared history must not obscure
// the independently retained diagnostic evidence.
export default function RepairHistoryPreview({ id, authFetch, t }) {
    const [open, setOpen] = useState(false)
    const [state, setState] = useState({})
    useEffect(() => {
        setState({})
        if (!open) return
        const controller = new AbortController()
        authFetch(`/admin/chat-history/${encodeURIComponent(id)}`, { signal: controller.signal })
            .then(readDiagnosticResponse)
            .then(body => { if (!controller.signal.aborted) setState({ record: body.item }) })
            .catch(error => { if (!controller.signal.aborted) setState({ error: error.message }) })
        return () => controller.abort()
    }, [open, id, authFetch])
    const record = state.record
    return <div className="space-y-2">
        <button type="button" className="text-xs text-primary underline underline-offset-2" onClick={() => setOpen(value => !value)}>{t(open ? 'toolDiagnostics.closeArchive' : 'toolDiagnostics.openArchive')}</button>
        {open && <div className="space-y-2 rounded border border-border p-3">
            {!record && !state.error && <p>{t('toolDiagnostics.loading')}</p>}
            {state.error && <p role="alert" className="text-muted-foreground">{t('toolDiagnostics.archiveUnavailable')} {state.error}</p>}
            {record && <><p>{record.model} · {record.status_code} · {record.elapsed_ms} ms</p>
                {record.error && <p className="text-destructive">{record.error}</p>}
                {record.reasoning_content && <details><summary>{t('toolDiagnostics.archiveThinking')}</summary><pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all">{record.reasoning_content}</pre></details>}
                <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-all">{record.content}</pre></>}
        </div>}
    </div>
}
