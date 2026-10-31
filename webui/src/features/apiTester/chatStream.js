// Consume complete SSE events, including CRLF, multiline data and an EOF tail.
export async function readChatStream(body, signal, onDelta) {
    const reader = body.getReader()
    const decoder = new TextDecoder()
    let pending = '', data = [], content = '', thinking = '', finishReason = null, done = false
    const event = () => {
        const value = data.join('\n')
        data = []
        if (!value) return
        if (value.trim() === '[DONE]') { done = true; return }
        const json = JSON.parse(value)
        if (json.error) {
            const detail = json.error
            const message = typeof detail === 'string' ? detail : detail.message
            const error = new Error(message || 'Upstream stream failed')
            error.statusCode = Number(json.status_code || detail.status_code || detail.http_status) || (detail.code === 'content_filter' ? 400 : 502)
            error.code = detail.code || ''
            error.type = detail.type || ''
            throw error
        }
        const choice = json.choices?.[0]
        if (choice?.finish_reason) finishReason = choice.finish_reason
        const delta = choice?.delta
        if (delta) {
            content += delta.content || ''
            thinking += delta.reasoning_content || ''
            onDelta(delta)
        }
    }
    const line = raw => {
        const value = raw.replace(/\r$/, '')
        if (value === '') { event(); return }
        if (value.startsWith('data:')) data.push(value.slice(5).replace(/^ /, ''))
    }
    const cancel = () => {
        reader.cancel().catch(error => console.warn('Stream cancellation failed:', error))
    }
    signal.addEventListener('abort', cancel, { once: true })
    try {
        if (signal.aborted) throw new DOMException('Aborted', 'AbortError')
        while (!done) {
            const chunk = await reader.read()
            if (signal.aborted) throw new DOMException('Aborted', 'AbortError')
            pending += chunk.done ? decoder.decode() : decoder.decode(chunk.value, { stream: true })
            let newline
            while (!done && (newline = pending.indexOf('\n')) >= 0) {
                const current = pending.slice(0, newline)
                pending = pending.slice(newline + 1)
                line(current)
            }
            if (chunk.done) {
                if (pending) line(pending)
                if (data.length) event()
                break
            }
        }
        if (!done && !finishReason) throw new Error('Upstream stream ended before completion')
        return { content, thinking, finishReason: finishReason || 'stop' }
    } finally {
        signal.removeEventListener('abort', cancel)
        try { await reader.cancel() } catch (error) { console.warn('Stream cleanup failed:', error) }
        reader.releaseLock()
    }
}
