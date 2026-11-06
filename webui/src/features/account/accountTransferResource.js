import { createRequestGroup } from '../../utils/latestRequest.js'

export const MAX_ACCOUNT_IMPORT_BYTES = 16 * 1024 * 1024

async function requireSuccess(response) {
    if (response.ok) return
    let detail = ''
    try { detail = (await response.json()).detail || '' } catch { /* Use the translated fallback. */ }
    throw Object.assign(new Error('Account transfer failed'), { detail })
}

// Import/export share one owner; a late response after logout or unmount must
// never download credentials, notify, or refresh a newly mounted account page.
export function createAccountTransferResource(onBusy) {
    const requests = createRequestGroup()
    const run = async (kind, action, callbacks) => {
        const request = requests.start('transfer')
        if (!request) return
        onBusy(kind)
        try {
            await action(request)
        } catch (error) {
            if (request.isCurrent()) callbacks.onError?.(error.detail || '')
        } finally {
            if (request.isCurrent()) {
                request.finish()
                onBusy('')
            }
        }
    }
    return {
        cancel: requests.cancel,
        exportAccounts(apiFetch, saveFile, callbacks = {}) {
            return run('export', async request => {
                const response = await apiFetch('/admin/accounts/export', { signal: request.signal })
                if (!request.isCurrent()) return
                await requireSuccess(response)
                const blob = await response.blob()
                if (!request.isCurrent()) return
                saveFile(blob)
                callbacks.onExported?.()
            }, callbacks)
        },
        importAccounts(file, apiFetch, callbacks = {}) {
            if (!file) return Promise.resolve()
            return run('import', async request => {
                if (file.size > MAX_ACCOUNT_IMPORT_BYTES) {
                    callbacks.onTooLarge?.()
                    return
                }
                // Send original file bytes; parsing/stringifying here could
                // alter stored timestamps, passwords or token strings.
                const response = await apiFetch('/admin/accounts/import', {
                    method: 'POST', headers: { 'Content-Type': 'application/json' },
                    body: file, signal: request.signal,
                })
                if (!request.isCurrent()) return
                await requireSuccess(response)
                const data = await response.json()
                if (!request.isCurrent()) return
                if (data.success !== true) throw new Error('Invalid account import response')
                await callbacks.onImported?.(data, request.isCurrent)
            }, callbacks)
        },
    }
}
