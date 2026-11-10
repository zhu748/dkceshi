import { createToolCallStream } from './toolCallStream.js'
import { createSseLines } from './chatSseLines.js'
import streamLimits from '../../../../internal/streambudget/defaults.json' with { type: 'json' }

// Consume complete SSE events, including CR/LF/CRLF and an EOF tail.
export async function readChatStream(body, signal, onDelta) {
    const reader = body.getReader()
    const decoder = new TextDecoder()
    const tools = createToolCallStream()
    let data = [], eventChars = 0, outputBytes = 0, content = '', thinking = '', finishReason = null, done = false
    const event = () => {
        const value = data.join('\n')
        data = []
        eventChars = 0
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
            onDelta(delta, tools.append(delta.tool_calls))
        }
    }
    const lines = createSseLines(value => {
        if (value === '') event()
        else if (value.startsWith('data:')) {
            const text = value.slice(5).replace(/^ /, '')
            eventChars += text.length + 1
            if (eventChars > streamLimits.max_line_bytes) throw new Error('SSE event exceeded the API tester limit')
            data.push(text)
        }
        return !done
    }, streamLimits.max_line_bytes)
    const cancel = () => {
        reader.cancel().catch(error => console.warn('Stream cancellation failed:', error))
    }
    signal.addEventListener('abort', cancel, { once: true })
    try {
        if (signal.aborted) throw new DOMException('Aborted', 'AbortError')
        while (!done) {
            const chunk = await reader.read()
            if (signal.aborted) throw new DOMException('Aborted', 'AbortError')
            outputBytes += chunk.value?.byteLength || 0
            if (outputBytes > streamLimits.max_output_bytes) throw new Error('Stream exceeded the API tester limit')
            lines.push(chunk.done ? decoder.decode() : decoder.decode(chunk.value, { stream: true }))
            if (chunk.done) {
                if (!done) lines.finish()
                if (!done && data.length) event()
                break
            }
        }
        if (!done && !finishReason) throw new Error('Upstream stream ended before completion')
        return { content, thinking, finishReason: finishReason || 'stop', toolCalls: tools.snapshot() }
    } finally {
        signal.removeEventListener('abort', cancel)
        try { await reader.cancel() } catch (error) { console.warn('Stream cleanup failed:', error) }
        reader.releaseLock()
    }
}
