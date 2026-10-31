import { createLatestRequest } from '../../utils/latestRequest.js'

// One modal session owns credential reads and saves. Closing or selecting a
// different account invalidates callbacks even if fetch/JSON ignores abort.
export function createManualTokenResource(onChange) {
    const requests = createLatestRequest()
    let selected = null

    const isCurrent = (session) => selected === session && session.request.isCurrent()
    const cancel = () => {
        requests.cancel()
        selected = null
    }
    const close = () => {
        cancel()
        onChange({ open: false, account: null, token: '', saving: false })
    }
    const offer = (account) => {
        const session = { account, request: requests.start(), saving: false }
        selected = session
        onChange({ open: true, account, token: '', saving: false })
        return session
    }

    const open = async (identifier, apiFetch) => {
        const session = offer({ identifier, username: identifier, password: '' })
        try {
            const res = await apiFetch(`/admin/accounts/${encodeURIComponent(identifier)}/credential`, {
                signal: session.request.signal,
            })
            if (!isCurrent(session) || !res.ok) return
            const data = await res.json()
            if (!isCurrent(session)) return
            session.account = {
                identifier,
                username: data.account_username || identifier,
                password: data.account_password || '',
            }
            onChange({ account: session.account })
        } catch {
            // Credential lookup is optional; manual entry stays available.
        }
    }

    const submit = async (token, apiFetch, events) => {
        const session = selected
        const value = String(token || '').trim()
        if (!session?.account?.identifier || !value) {
            events.onRequired()
            return
        }
        if (session.saving) return
        session.saving = true
        onChange({ saving: true })
        try {
            const res = await apiFetch(`/admin/accounts/${encodeURIComponent(session.account.identifier)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ token: value }),
                signal: session.request.signal,
            })
            if (!isCurrent(session)) return
            const data = await res.json()
            if (!isCurrent(session)) return
            if (!res.ok) {
                events.onRejected(data.detail)
                return
            }
            close()
            events.onSaved()
        } catch {
            if (isCurrent(session)) events.onError()
        } finally {
            if (isCurrent(session)) {
                session.saving = false
                onChange({ saving: false })
            }
        }
    }

    return { open, offer, close, cancel, submit }
}
