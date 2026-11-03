import { createLatestRequest } from '../../utils/latestRequest.js'
import { entryToRecord } from './usageUtils.js'

// A validator belongs to the last accepted snapshot. Stale responses and
// failed reads must never publish a validator for data the UI does not own.
export function createUsageRecordsResource({ onChange, onError }) {
    const requests = createLatestRequest()
    let etag = ''
    let disposed = false
    return {
        activate() { disposed = false },
        dispose() { disposed = true; requests.cancel() },
        clear() {
            if (disposed) return
            requests.cancel()
            etag = ''
            onChange({ records: [], loading: false, initialized: true, error: '' })
        },
        async load(authFetch, fallbackError) {
            if (disposed) return
            const request = requests.start()
            const current = () => !disposed && request.isCurrent()
            onChange({ loading: true, error: '' })
            try {
                const res = await authFetch('/admin/usage', {
                    signal: request.signal,
                    headers: etag ? { 'If-None-Match': etag } : {},
                })
                if (!current() || res.status === 304) return
                const data = await res.json().catch(() => ({}))
                if (!current()) return
                if (!res.ok) throw new Error(data?.detail || fallbackError)
                const items = Array.isArray(data.items) ? data.items : []
                const records = items.map(entryToRecord).filter((r) => r.created_at > 0)
                etag = res.headers?.get('ETag') || ''
                onChange({ records })
            } catch (error) {
                if (current()) {
                    const message = error.message || fallbackError
                    onChange({ error: message })
                    onError?.(message)
                }
            } finally {
                if (current()) onChange({ loading: false, initialized: true })
            }
        },
    }
}
