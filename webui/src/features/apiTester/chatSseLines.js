// Scan each chunk once, retaining unfinished lines as fragments. Searching the
// entire accumulated tail on each read makes a large fragmented event quadratic.
export function createSseLines(onLine, maxLineChars) {
    let parts = [], length = 0, skipLF = false
    const append = text => {
        length += text.length
        if (length > maxLineChars) throw new Error('SSE line exceeded the API tester limit')
        if (text) parts.push(text)
    }
    const emit = (tail = '') => {
        if (length + tail.length > maxLineChars) throw new Error('SSE line exceeded the API tester limit')
        const line = parts.length ? parts.join('') + tail : tail
        if (parts.length) parts = []
        length = 0
        return onLine(line) !== false
    }
    return {
        push(text) {
            if (!text) return true
            let start = 0
            if (skipLF) {
                skipLF = false
                if (text.charCodeAt(0) === 10) start = 1
            }
            const endings = /[\r\n]/g
            endings.lastIndex = start
            let match
            while ((match = endings.exec(text))) {
                if (!emit(text.slice(start, match.index))) return false
                start = endings.lastIndex
                if (match[0] === '\r') {
                    if (text.charCodeAt(start) === 10) endings.lastIndex = ++start
                    else if (start === text.length) skipLF = true
                }
            }
            append(text.slice(start))
            return true
        },
        finish() { if (length) return emit(); return true },
    }
}
