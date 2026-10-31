import { createLatestRequest } from '../../utils/latestRequest.js'
import { getAttachedFileAccountIds } from './fileAccountBinding.js'
import { readChatStream } from './chatStream.js'

async function responseError(response, fallback) {
    const raw = await response.text()
    if (!raw) return fallback
    try {
        const data = JSON.parse(raw)
        const candidates = [data.error?.message, data.error, data.detail, data.message]
        return candidates.find(value => typeof value === 'string' && value.trim()) || fallback
    } catch { return raw.slice(0, 240) }
}

// Own request identity independently of React. Late fetch/JSON/read/finally
// results cannot write into a newer request or a component that has unmounted.
export function createChatRequest(state, fetcher = globalThis.fetch) {
    const requests = createLatestRequest()
    let running = false
    const cancel = () => { requests.cancel(); running = false }
    const stop = () => {
        const wasRunning = running
        cancel()
        state.setLoading(false)
        state.setIsStreaming(false)
        return wasRunning
    }
    const run = async ({ t, onMessage, model, message, effectiveKey, selectedAccount, streamingMode, attachedFiles = [] }) => {
        if (!effectiveKey) { onMessage('error', t('apiTester.missingApiKey')); return }
        const request = requests.start(), started = Date.now()
        running = true
        state.setLoading(true)
        state.setIsStreaming(Boolean(streamingMode))
        state.setResponse(null)
        state.setStreamingContent('')
        state.setStreamingThinking('')
        try {
            const ids = getAttachedFileAccountIds(attachedFiles)
            if (ids.length > 1) throw new Error(t('apiTester.fileAccountConflict'))
            const selected = String(selectedAccount || '').trim()
            if (ids[0] && selected && ids[0] !== selected) throw new Error(t('apiTester.fileAccountMismatch', { account: ids[0] }))
            const account = selected || ids[0] || ''
            const headers = { 'Content-Type': 'application/json', Authorization: `Bearer ${effectiveKey}` }
            if (account) headers['X-Ds2-Target-Account'] = account
            const body = { model, messages: [{ role: 'user', content: message }], stream: streamingMode }
            if (attachedFiles.length) body.file_ids = attachedFiles.map(file => file.id)
            const response = await fetcher(streamingMode ? '/v1/chat/completions' : '/v1/chat/completions?__go=1', {
                method: 'POST', headers, body: JSON.stringify(body), signal: request.signal,
            })
            if (!request.isCurrent()) {
                try { await response.body?.cancel() } catch (error) { console.warn('Stale response cleanup failed:', error) }
                return
            }
            if (!response.ok) {
                const error = new Error(await responseError(response, t('apiTester.requestFailed')))
                error.statusCode = response.status
                throw error
            }
            if (streamingMode) {
                state.setResponse({ success: true, status_code: response.status })
                const result = await readChatStream(response.body, request.signal, delta => {
                    if (!request.isCurrent()) return
                    if (delta.content) state.setStreamingContent(previous => previous + delta.content)
                    if (delta.reasoning_content) state.setStreamingThinking(previous => previous + delta.reasoning_content)
                })
                if (!request.isCurrent()) return
                state.setResponse({ success: true, status_code: response.status, choices: [{
                    index: 0, finish_reason: result.finishReason,
                    message: { role: 'assistant', content: result.content, reasoning_content: result.thinking },
                }] })
            } else {
                const data = await response.json()
                if (!request.isCurrent()) return
                state.setResponse({ ...data, success: true, status_code: response.status })
            }
            if (request.isCurrent()) onMessage('success', t('apiTester.requestSuccess', { account: account || 'Auto', time: Date.now() - started }))
        } catch (error) {
            if (!request.isCurrent()) return
            state.setStreamingContent('')
            state.setStreamingThinking('')
            state.setResponse({ success: false, error: error.message, status_code: error.statusCode, code: error.code, type: error.type })
            onMessage('error', error.message)
        } finally {
            if (request.isCurrent()) {
                running = false
                state.setLoading(false)
                state.setIsStreaming(false)
            }
        }
    }
    return { run, stop, cancel }
}
