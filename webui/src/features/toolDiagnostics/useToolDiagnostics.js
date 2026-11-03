import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createDiagnosticRequests, readDiagnosticResponse } from './diagnosticRequests'
import { createDiagnosticPoll } from './diagnosticPoll'

const ENDPOINT = '/admin/tool-call-records'

export function useToolDiagnostics(authFetch, onMessage) {
    const [requests] = useState(createDiagnosticRequests)
    const [query, setQuery] = useState({ page: 1, status: '', search: '', pinned: false })
    const [search, setSearch] = useState('')
    useEffect(() => { const timer = setTimeout(() => setSearch(query.search), 250); return () => clearTimeout(timer) }, [query.search])
    const requestQuery = useMemo(() => ({ page: query.page, status: query.status, pinned: query.pinned, search }), [query.page, query.status, query.pinned, search])
    const [data, setData] = useState(null)
    const [selected, setSelected] = useState('')
    const [detail, setDetail] = useState(null)
    const detailCache = useRef(null)
    const [listError, setListError] = useState('')
    const [detailError, setDetailError] = useState('')
    const [busy, setBusy] = useState(false)
    const [revision, setRevision] = useState(0)
    const refresh = useCallback(() => setRevision(value => value + 1), [])

    useEffect(() => {
        requests.activate()
        detailCache.current = null
        setData(null)
        setDetail(null)
        setSelected('')
        setBusy(false)
        return () => requests.cancel()
    }, [authFetch, requests])

    useEffect(() => {
        const controller = new AbortController()
        const load = async () => {
            const body = await requests.list(authFetch, requestQuery, controller.signal)
            if (!body) return
            setData(body)
            setListError('')
            if (!body.items.length && requestQuery.page > 1) {
                setQuery(previous => ({ ...previous, page: previous.page - 1 }))
            } else {
                setSelected(previous => body.items.some(item => item.id === previous) ? previous : body.items[0]?.id || '')
            }
        }
        const poll = createDiagnosticPoll({ load, isVisible: () => document.visibilityState === 'visible', onError: failure => { if (!controller.signal.aborted) setListError(failure.message) } })
        poll.start()
        document.addEventListener('visibilitychange', poll.visibilityChanged)
        return () => { controller.abort(); poll.stop(); document.removeEventListener('visibilitychange', poll.visibilityChanged) }
    }, [authFetch, requestQuery, revision, requests])

    const selectedRevision = data?.items.find(item => item.id === selected)?.revision
    useEffect(() => {
        const controller = new AbortController()
        setDetailError('')
        if (!selected) { setDetail(null); return () => controller.abort() }
        setDetail(previous => previous?.id === selected ? previous : null)
        const cached = detailCache.current?.id === selected ? detailCache.current : null
        authFetch(`${ENDPOINT}/${encodeURIComponent(selected)}`, { signal: controller.signal, ...(cached?.etag ? { headers: { 'If-None-Match': cached.etag } } : {}) })
            .then(async response => {
                if (response.status === 304 && cached) return cached.record
                const body = await readDiagnosticResponse(response)
                if (!controller.signal.aborted) detailCache.current = { id: selected, etag: response.headers?.get('ETag'), record: body }
                return body
            })
            .then(body => { if (!controller.signal.aborted) setDetail(body) })
            .catch(failure => { if (!controller.signal.aborted) setDetailError(failure.message) })
        return () => controller.abort()
    }, [authFetch, selected, selectedRevision, revision])

    const mutate = async (path, method, body) => {
        if (requests.busy) return false
        const generation = requests.generation
        setBusy(true)
        try {
            const saved = await requests.mutate(authFetch, path, method, body)
            if (!saved || !requests.isCurrent(generation)) return false
            if (saved.settings) setData(previous => previous ? { ...previous, settings: saved.settings, revision: saved.revision } : previous)
            refresh()
            return saved
        } catch (failure) {
            if (requests.isCurrent(generation)) onMessage?.('error', failure.message)
            return false
        } finally { if (requests.isCurrent(generation)) setBusy(requests.busy) }
    }
    return { data, query, setQuery, selected, setSelected, detail, error: listError || detailError, busy, refresh, mutate }
}
