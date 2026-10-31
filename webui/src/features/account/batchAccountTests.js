import { createLatestRequest } from '../../utils/latestRequest.js'

// One batch owns its pending request and progress. Canceling a batch prevents
// late responses/JSON from publishing state or starting another account test.
export function createBatchAccountTests({ onRunning, onProgress }) {
    const requests = createLatestRequest()
    let active = null

    const dispose = () => {
        requests.cancel()
        active = null
    }
    const cancel = () => {
        if (!active) return null
        const progress = active.progress
        dispose()
        onRunning(false)
        return progress
    }
    const run = (options) => {
        if (active) return active.promise
        const identifiers = options.accounts.map(account => String(options.resolveAccountIdentifier(account) || '').trim())
        if (!identifiers.length) return Promise.resolve()
        const current = {
            request: requests.start(), promise: null,
            progress: { current: 0, total: identifiers.length, results: [] },
        }
        active = current
        onRunning(true)
        onProgress(current.progress)
        current.promise = Promise.resolve().then(async () => {
            const results = []
            let success = 0
            try {
                for (const [index, identifier] of identifiers.entries()) {
                    if (!current.request.isCurrent()) return
                    let result = { id: '-', success: false, message: options.invalidIdentifier }
                    if (identifier) {
                        try {
                            const response = await options.apiFetch('/admin/accounts/test', {
                                method: 'POST',
                                headers: { 'Content-Type': 'application/json' },
                                body: JSON.stringify({ identifier }),
                                signal: current.request.signal,
                            })
                            if (!current.request.isCurrent()) return
                            const data = await response.json()
                            if (!current.request.isCurrent()) return
                            result = { id: identifier, success: data.success, message: data.message, time: data.response_time }
                            if (data.success) success++
                        } catch (error) {
                            if (!current.request.isCurrent()) return
                            result = { id: identifier, success: false, message: error.message }
                        }
                    }
                    results.push(result)
                    current.progress = { current: index + 1, total: identifiers.length, results: [...results] }
                    onProgress(current.progress)
                }
                if (current.request.isCurrent()) options.onCompleted({ success, total: identifiers.length })
            } finally {
                if (current.request.isCurrent()) {
                    active = null
                    onRunning(false)
                }
            }
        })
        return current.promise
    }

    return { run, cancel, dispose, get running() { return active !== null } }
}
