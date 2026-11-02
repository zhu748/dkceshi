import { createRequestGroup } from '../../utils/latestRequest.js'
import { repairDraftBody } from './repairDraft.js'

export function createRepairTestResource() {
    const requests = createRequestGroup()
    return {
        cancel: requests.cancel,
        async run(authFetch, draft, commit) {
            const request = requests.start('repair-test')
            if (!request) return
            commit({ loading: true, result: null, error: '' })
            try {
                const response = await authFetch('/admin/tool-call-records/repair-test', {
                    method: 'POST', signal: request.signal,
                    headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(repairDraftBody(draft)),
                })
                const body = await response.json()
                if (!request.isCurrent()) return
                if (!response.ok) throw new Error(body.detail || `HTTP ${response.status}`)
                commit({ loading: false, result: body, error: '' })
            } catch (error) {
                if (request.isCurrent()) commit({ loading: false, result: null, error: error.message })
            } finally { request.finish() }
        },
    }
}
