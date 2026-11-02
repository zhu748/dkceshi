import { useCallback, useEffect, useState } from 'react'

const ENDPOINT = '/admin/tool-call-records'

async function readResponse(response) {
    const body = await response.json()
    if (!response.ok) throw new Error(body.detail || `HTTP ${response.status}`)
    return body
}

export function useToolDiagnostics(authFetch, onMessage) {
    const [query, setQuery] = useState({ page: 1, status: '', search: '', pinned: false })
    const [data, setData] = useState(null)
    const [selected, setSelected] = useState('')
    const [detail, setDetail] = useState(null)
    const [error, setError] = useState('')
    const [busy, setBusy] = useState(false)
    const [revision, setRevision] = useState(0)
    const refresh = useCallback(() => setRevision(value => value + 1), [])

    useEffect(() => {
        const controller = new AbortController()
        let timer
        const load = async () => {
            try {
                const params = new URLSearchParams({ ...query, page_size: 20 })
                const body = await readResponse(await authFetch(`${ENDPOINT}?${params}`, { signal: controller.signal }))
                if (controller.signal.aborted) return
                setData(body)
                setError('')
                if (!body.items.length && query.page > 1) {
                    setQuery(previous => ({ ...previous, page: previous.page - 1 }))
                } else {
                    setSelected(previous => body.items.some(item => item.id === previous) ? previous : body.items[0]?.id || '')
                }
            } catch (failure) {
                if (!controller.signal.aborted) setError(failure.message)
            } finally {
                if (!controller.signal.aborted) timer = setTimeout(() => {
                    if (document.visibilityState === 'visible') load()
                    else timer = setTimeout(load, 5000)
                }, 2500)
            }
        }
        load()
        return () => { controller.abort(); clearTimeout(timer) }
    }, [authFetch, query, revision])

    const selectedRevision = data?.items.find(item => item.id === selected)?.revision
    useEffect(() => {
        const controller = new AbortController()
        if (!selected) { setDetail(null); return () => controller.abort() }
        setDetail(previous => previous?.id === selected ? previous : null)
        authFetch(`${ENDPOINT}/${encodeURIComponent(selected)}`, { signal: controller.signal })
            .then(readResponse)
            .then(body => { if (!controller.signal.aborted) setDetail(body) })
            .catch(failure => { if (!controller.signal.aborted) setError(failure.message) })
        return () => controller.abort()
    }, [authFetch, selected, selectedRevision, revision])

    const mutate = async (path, method, body) => {
        if (busy) return false
        setBusy(true)
        try {
            await readResponse(await authFetch(`${ENDPOINT}${path}`, {
                method,
                ...(body === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
            }))
            refresh()
            return true
        } catch (failure) {
            onMessage?.('error', failure.message)
            return false
        } finally { setBusy(false) }
    }

    return { data, query, setQuery, selected, setSelected, detail, error, busy, refresh, mutate }
}
