// Tool arguments are source text until the caller executes the finalized turn.
// Assemble by index, retaining IDs and arbitrary argument fragment boundaries.
export function createToolCallStream() {
    const calls = new Map()
    return {
        append(deltas) {
            if (!Array.isArray(deltas)) return undefined
            for (const delta of deltas) {
                const index = delta.index ?? 0
                if (!Number.isInteger(index) || index < 0) continue
                const call = calls.get(index) || { type: 'function', function: { name: '', arguments: '' } }
                if (delta.id) call.id = delta.id
                if (delta.type) call.type = delta.type
                if (typeof delta.function?.name === 'string') call.function.name += delta.function.name
                if (typeof delta.function?.arguments === 'string') call.function.arguments += delta.function.arguments
                calls.set(index, call)
            }
            return this.snapshot()
        },
        snapshot() {
            return [...calls].sort(([a], [b]) => a - b).map(([, call]) => ({ ...call, function: { ...call.function } }))
        },
    }
}
