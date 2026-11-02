import { createLatestRequest } from '../../utils/latestRequest.js'

export function emptyRepairModelState() {
    return { models: [], loading: false, fetched: false, truncated: false, error: '' }
}

export function createRepairModelResource() {
    const requests = createLatestRequest()
    return {
        cancel: requests.cancel,
        async load(authFetch, draft, commit) {
            const request = requests.start()
            commit({ loading: true, error: '' })
            try {
                const response = await authFetch('/admin/tool-call-records/repair-models', {
                    method: 'POST', signal: request.signal,
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ mode: 'custom', protocol: draft.protocol, base_url: draft.base_url, api_key: draft.api_key, clear_api_key: draft.clear_api_key, timeout_seconds: draft.timeout_seconds }),
                })
                const body = await response.json()
                if (!request.isCurrent()) return
                if (!response.ok) throw new Error(body.detail || `HTTP ${response.status}`)
                commit({ models: Array.isArray(body.models) ? body.models : [], loading: false, fetched: true, truncated: Boolean(body.truncated), error: '' })
            } catch (error) {
                if (request.isCurrent()) commit({ loading: false, error: error.message })
            }
        },
    }
}
