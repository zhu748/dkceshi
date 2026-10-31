// Only the latest request may commit state, even when a fetch implementation
// ignores AbortSignal or an already-started JSON parse finishes after cancel.
export function createLatestRequest() {
    let active = null
    const cancel = () => {
        active?.abort()
        active = null
    }
    const start = () => {
        cancel()
        const controller = new AbortController()
        active = controller
        return {
            signal: controller.signal,
            isCurrent: () => active === controller && !controller.signal.aborted,
        }
    }
    return { start, cancel }
}

// Independent operations may run together, while duplicate keys share one
// owner. Finishing an old operation cannot remove a newer operation's state.
export function createRequestGroup() {
    const active = new Map()
    const cancel = () => {
        for (const controller of active.values()) controller.abort()
        active.clear()
    }
    const start = key => {
        if (active.has(key)) return null
        const controller = new AbortController()
        active.set(key, controller)
        return {
            signal: controller.signal,
            isCurrent: () => active.get(key) === controller && !controller.signal.aborted,
            finish: () => { if (active.get(key) === controller) active.delete(key) },
        }
    }
    return { start, cancel, has: key => active.has(key), get size() { return active.size } }
}
