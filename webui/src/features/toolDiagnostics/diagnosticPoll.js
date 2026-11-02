// One request at a time; hidden tabs stop scheduling and resume immediately.
export function createDiagnosticPoll({ load, isVisible, onError, setTimer = setTimeout, clearTimer = clearTimeout, interval = 2500 }) {
    let stopped = true, running = false, timer = null
    const clear = () => { if (timer !== null) clearTimer(timer); timer = null }
    const run = async () => {
        if (stopped || running || !isVisible()) return
        clear()
        running = true
        try { await load() }
        catch (error) { if (!stopped) onError?.(error) }
        finally {
            running = false
            if (!stopped && isVisible()) timer = setTimer(run, interval)
        }
    }
    return {
        start() { stopped = false; return run() },
        visibilityChanged() { clear(); return run() },
        stop() { stopped = true; clear() },
    }
}
