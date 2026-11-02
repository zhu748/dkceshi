const ENDPOINT = '/admin/tool-call-records'
export async function readDiagnosticResponse(response) {
    const body = await response.json()
    if (!response.ok) throw new Error(body.detail || `HTTP ${response.status}`)
    return body
}
// Separate the current session from polling revisions. Late saves cannot update
// a new session or release its mutation lock, even when fetch ignores abort.
export function createDiagnosticRequests() {
    let epoch = 0, generation = 0, busy = false, active = true, mutation = null
    const isCurrent = version => active && generation === version
    return {
        get busy() { return busy },
        get generation() { return generation },
        isCurrent,
        activate() { active = true; generation++; epoch++ },
        cancel() { active = false; generation++; epoch++; busy = false; mutation?.abort(); mutation = null },
        async list(authFetch, query, signal) {
            if (!active || signal.aborted) return null
            const version = epoch
            const params = new URLSearchParams({ ...query, page_size: 20 })
            try {
                const body = await readDiagnosticResponse(await authFetch(`${ENDPOINT}?${params}`, { signal }))
                return active && !signal.aborted && version === epoch && !busy ? body : null
            } catch (error) {
                if (active && !signal.aborted && version === epoch && !busy) throw error
                return null
            }
        },
        async mutate(authFetch, path, method, body) {
            if (!active || busy) return false
            const version = generation
            const controller = new AbortController()
            mutation = controller
            busy = true
            epoch++
            try {
                const saved = await readDiagnosticResponse(await authFetch(`${ENDPOINT}${path}`, {
                    method, signal: controller.signal,
                    ...(body === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
                }))
                return isCurrent(version) ? saved : false
            } catch (error) {
                if (isCurrent(version)) throw error
                return false
            } finally {
                if (isCurrent(version)) { epoch++; busy = false; mutation = null }
            }
        },
    }
}
