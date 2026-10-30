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
