// Requests never overlap. Every completed attempt schedules its successor,
// including a success that did not change any React effect dependency.
export function createStatusPoll({ load, onState, onError, isVercel, setTimer = setTimeout, clearTimer = clearTimeout }) {
    let stopped = true, timer = null, pending = null, failures = 0, controller
    const clear = () => { if (timer !== null) clearTimer(timer); timer = null }
    const delay = () => !isVercel || !failures ? 15000 : failures === 1 ? 30000 : 60000
    const refresh = ({ manual = false } = {}) => {
        if (stopped) return Promise.resolve()
        if (pending) return pending
        clear()
        if (manual) failures = 0
        controller = new AbortController()
        const request = controller
        const current = () => !stopped && controller === request
        pending = Promise.resolve().then(() => load(request.signal)).then(data => {
            if (!current()) return
            failures = 0
            onState({ syncStatus: data, pollFailures: 0, pollPaused: false })
        }).catch(error => {
            if (!current()) return
            failures++
            onState({ pollFailures: failures, pollPaused: isVercel && failures >= 3 })
            if (manual) onError?.(error)
        }).finally(() => {
            if (!current()) return
            pending = null
            if (!isVercel || failures < 3) timer = setTimer(() => refresh(), delay())
        })
        return pending
    }
    return {
        start() { stopped = false; return refresh() },
        refresh,
        stop() { stopped = true; clear(); controller?.abort(); controller = null; pending = null },
    }
}
