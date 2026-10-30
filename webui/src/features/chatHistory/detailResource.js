// Own one selected detail request and its ETag. Switching selection cancels
// the old request; identity checks also protect against fetchers ignoring abort.
export function createDetailResource() {
    let active = null
    let selectedId = ''
    let etag = ''

    const cancel = () => {
        active?.controller.abort()
        active = null
    }

    const invalidate = () => {
        cancel()
        selectedId = ''
        etag = ''
    }

    const load = (id, apiFetch, accept, fallbackError) => {
        if (!id) return Promise.resolve()
        if (active?.id === id) return active.promise
        cancel()
        if (selectedId !== id) {
            selectedId = id
            etag = ''
        }
        const request = { id, controller: new AbortController(), promise: null }
        active = request
        request.promise = (async () => {
            try {
                const headers = etag ? { 'If-None-Match': etag } : {}
                const res = await apiFetch(`/admin/chat-history/${encodeURIComponent(id)}`, {
                    headers, signal: request.controller.signal,
                })
                if (active !== request || res.status === 304) return
                const data = await res.json()
                if (active !== request) return
                if (!res.ok) throw new Error(data?.detail || fallbackError)
                etag = res.headers.get('ETag') || ''
                accept(data.item || null)
            } catch (error) {
                if (active === request && !request.controller.signal.aborted) throw error
            } finally {
                if (active === request) active = null
            }
        })()
        return request.promise
    }

    return { load, cancel, invalidate }
}
