import { useEffect, useRef, useState } from 'react'
import { createLatestRequest, createRequestGroup } from '../../utils/latestRequest.js'

const ENDPOINT = '/admin/chat-history'
const emptyState = source => ({ source, items: [], limit: 20, loading: true, refreshing: false,
    savingLimit: false, clearing: false, deletingId: '', detail: '', autoRefreshReady: false })

// Own list validators, polling and mutations together so a late poll cannot
// undo a successful delete/clear/retention change or cross request sources.
export function useChatHistoryList({ apiFetch, t, onMessage, onMutation }) {
    const [state, setState] = useState(() => emptyState(apiFetch))
    const [requests] = useState(createLatestRequest)
    const [mutations] = useState(createRequestGroup)
    const pending = useRef(null)
    const etag = useRef('')
    const options = useRef(null)
    options.current = { t, onMessage, onMutation }
    const poll = useRef(null)

    const loadList = ({ mode = 'silent', announceError = false } = {}) => {
        if (mutations.size) return Promise.resolve()
        if (pending.current) return pending.current.promise
        const request = requests.start()
        const operation = { promise: null }
        pending.current = operation
        setState(previous => ({ ...previous,
            loading: mode === 'initial' || previous.loading,
            refreshing: mode === 'manual', detail: announceError ? '' : previous.detail }))
        operation.promise = (async () => {
            try {
                const response = await apiFetch(ENDPOINT, {
                    signal: request.signal, headers: etag.current ? { 'If-None-Match': etag.current } : {},
                })
                if (!request.isCurrent()) return
                if (response.status === 304) {
                    if (!etag.current) throw new Error(options.current.t('chatHistory.loadFailed'))
                    setState(previous => ({ ...previous, detail: '' }))
                    return
                }
                const data = await response.json()
                if (!request.isCurrent()) return
                if (!response.ok || !Array.isArray(data?.items)) {
                    throw new Error(data?.detail || options.current.t('chatHistory.loadFailed'))
                }
                etag.current = response.headers?.get('ETag') || ''
                setState(previous => ({ ...previous, items: data.items,
                    limit: typeof data.limit === 'number' ? data.limit : 20, detail: '' }))
            } catch (error) {
                if (!request.isCurrent()) return
                const message = error.message || options.current.t('chatHistory.loadFailed')
                setState(previous => ({ ...previous, detail: message }))
                if (announceError) options.current.onMessage?.('error', message)
            } finally {
                if (request.isCurrent()) {
                    pending.current = null
                    setState(previous => ({ ...previous, loading: false, refreshing: false, autoRefreshReady: true }))
                }
            }
        })()
        return operation.promise
    }
    poll.current = loadList

    useEffect(() => {
        etag.current = ''
        setState(emptyState(apiFetch))
        loadList({ mode: 'initial', announceError: true })
        return () => {
            requests.cancel()
            mutations.cancel()
            pending.current = null
        }
    }, [apiFetch, requests, mutations])

    useEffect(() => {
        if (!state.autoRefreshReady || state.limit === 0) return undefined
        const timer = setInterval(() => poll.current?.(), 1500)
        return () => clearInterval(timer)
    }, [state.autoRefreshReady, state.limit])

    const mutate = async (kind, id, nextLimit) => {
        const request = mutations.start('history')
        if (!request) return
        // A write owns the next visible list state. Drop both a pending poll
        // and its validator before starting it; polls pause until it settles.
        requests.cancel()
        pending.current = null
        etag.current = ''
        setState(previous => ({ ...previous, loading: false, refreshing: false,
            savingLimit: kind === 'limit', clearing: kind === 'clear', deletingId: kind === 'delete' ? id : '' }))
        const { t: translate } = options.current
        const errorKey = kind === 'limit' ? 'updateLimitFailed' : kind === 'delete' ? 'deleteFailed' : 'clearFailed'
        try {
            const path = kind === 'limit' ? `${ENDPOINT}/settings`
                : kind === 'delete' ? `${ENDPOINT}/${encodeURIComponent(id)}` : ENDPOINT
            const response = await apiFetch(path, {
                method: kind === 'limit' ? 'PUT' : 'DELETE', signal: request.signal,
                ...(kind === 'limit' ? { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ limit: nextLimit }) } : {}),
            })
            if (!request.isCurrent()) return
            const data = await response.json()
            if (!request.isCurrent()) return
            if (!response.ok || (kind === 'limit' && !Array.isArray(data?.items))) {
                throw new Error(data?.detail || translate(`chatHistory.${errorKey}`))
            }
            const limit = typeof data.limit === 'number' ? data.limit : nextLimit
            setState(previous => ({ ...previous, detail: '',
                ...(kind === 'limit' ? { items: data.items, limit }
                    : { items: kind === 'clear' ? [] : previous.items.filter(item => item.id !== id) }) }))
            options.current.onMutation?.({ kind, id })
            const successKey = kind === 'limit' ? (limit === 0 ? 'disabledSuccess' : 'limitUpdated')
                : kind === 'delete' ? 'deleteSuccess' : 'clearSuccess'
            options.current.onMessage?.('success', options.current.t(`chatHistory.${successKey}`, { limit }))
        } catch (error) {
            if (request.isCurrent()) options.current.onMessage?.('error', error.message || options.current.t(`chatHistory.${errorKey}`))
        } finally {
            if (request.isCurrent()) {
                request.finish()
                setState(previous => ({ ...previous, savingLimit: false, clearing: false, deletingId: '', autoRefreshReady: true }))
            }
        }
    }

    // A new source must not render the previous source's data while its
    // cleanup/setup effects are still waiting to run.
    const visible = state.source === apiFetch ? state : emptyState(apiFetch)
    return { ...visible, busy: visible.savingLimit || visible.clearing || Boolean(visible.deletingId), loadList,
        handleLimitChange: next => next !== visible.limit ? mutate('limit', '', next) : undefined,
        handleDeleteItem: id => id ? mutate('delete', id) : undefined,
        handleClear: () => visible.items.length ? mutate('clear') : undefined }
}
