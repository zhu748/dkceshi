const SERIALIZED_HEADER = /^Serialized conversation\. Each message ends with (\[EndMessage:[a-f0-9]{16}\]_*)\./m

export function isSerializedHistory(text) {
    return SERIALIZED_HEADER.test(text)
}

// The delimiter is chosen by the server so it cannot occur in a message body.
// Headings and role labels inside bodies therefore remain literal content.
export function parseSerializedHistory(text) {
    const header = SERIALIZED_HEADER.exec(text)
    if (!header) return null
    const start = text.indexOf('\n\n', header.index)
    if (start < 0) return null
    const delimiter = `\n${header[1]}\n\n`
    const parts = text.slice(start + 2).split(delimiter)
    if (parts.pop() !== '') return null
    const messages = []
    for (const part of parts) {
        const match = /^=== (\d+)\. ([A-Z][A-Z_ -]*) ===\n(?:Message metadata: ([^\n]+)\n)?\[([A-Za-z]+)\]:/.exec(part)
        if (!match || Number(match[1]) !== messages.length + 1) return null
        const role = match[2].toLowerCase()
        if (match[4].toLowerCase() !== role && match[4] !== 'Unknown') return null
        let metadata = {}
        try {
            if (match[3]) metadata = JSON.parse(match[3])
        } catch {
            return null
        }
        if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) return null
        const message = { role, content: part.slice(match[0].length) }
        for (const key of ['name', 'id', 'tool_call_id', 'is_error']) {
            if (Object.hasOwn(metadata, key)) message[key] = metadata[key]
        }
        messages.push(message)
    }
    return messages.length ? messages : null
}

const CURRENT_CONTINUATION = '继续会话。HISTORY.txt contains earlier conversation context in chronological order. Follow the current system/developer instructions and continue the active user task and tool results below. If no user task follows here, the current user input is in HISTORY.txt. Match tool results by tool_call_id; a call without a matching result is not known to have completed. Do not repeat completed operations or mention the context filenames in your answer.'
const TOOLS_CONTINUATION = ' TOOLS.txt is the authoritative catalog and JSON Schemas for client-provided tools in this request. Emit client tool calls only for those tools, using their declared argument types and the live tool-choice and output-format rules. Upstream native capabilities, when enabled, are separate and do not use the client tool-call format.'

export function isCurrentContinuation(text) {
    return text === CURRENT_CONTINUATION || text === CURRENT_CONTINUATION + TOOLS_CONTINUATION
}
