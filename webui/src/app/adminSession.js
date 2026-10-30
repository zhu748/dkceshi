import { createLatestRequest } from '../utils/latestRequest.js'

const TOKEN_KEY = 'ds2api_token'
const EXPIRY_KEY = 'ds2api_token_expires'

function browserStorage(name) {
    try { return globalThis[name] } catch { return null }
}

export function readStoredAdminSession(local = browserStorage('localStorage'), session = browserStorage('sessionStorage'), now = Date.now()) {
    // Keep each token paired with its own expiration. A tab-only login takes
    // precedence over a stale remembered login from an earlier session.
    for (const storage of [session, local]) {
        try {
            const token = storage?.getItem(TOKEN_KEY)
            const expires = Number(storage?.getItem(EXPIRY_KEY))
            if (token && Number.isFinite(expires) && expires > now) return token
        } catch { /* A storage backend may be unavailable in privacy mode. */ }
    }
    return null
}

export function clearStoredAdminSession(local = browserStorage('localStorage'), session = browserStorage('sessionStorage')) {
    for (const storage of [local, session]) {
        for (const key of [TOKEN_KEY, EXPIRY_KEY]) {
            try { storage?.removeItem(key) } catch { /* In-memory logout must still work. */ }
        }
    }
}

export function saveStoredAdminSession(token, expiresIn, remember, local = browserStorage('localStorage'), session = browserStorage('sessionStorage')) {
    clearStoredAdminSession(local, session)
    const storage = remember ? local : session
    try {
        if (!storage) return false
        storage.setItem(TOKEN_KEY, token)
        storage.setItem(EXPIRY_KEY, String(Date.now() + expiresIn * 1000))
        return true
    } catch {
        clearStoredAdminSession(local, session)
        return false
    }
}

export function createAdminSession({ fetcher = globalThis.fetch, onToken, onChecking, local, session }) {
    const requests = createLatestRequest()
    let currentToken = null
    const publish = (token) => {
        currentToken = token
        onToken(token)
    }
    const logout = (expectedToken) => {
        if (typeof expectedToken === 'string' && currentToken !== expectedToken) return false
        requests.cancel()
        publish(null)
        onChecking(false)
        clearStoredAdminSession(local, session)
        return true
    }
    const login = (token) => {
        requests.cancel()
        publish(token)
        onChecking(false)
    }
    const restore = async () => {
        const request = requests.start()
        onChecking(true)
        const token = currentToken || readStoredAdminSession(local, session)
        try {
            if (!token) {
                publish(null)
                return
            }
            const res = await fetcher('/admin/verify', {
                headers: { Authorization: `Bearer ${token}` }, signal: request.signal,
            })
            if (!request.isCurrent()) return
            if (res.ok) publish(token)
            else logout()
        } catch {
            // Preserve the established offline fallback, but only for a
            // verification that still belongs to the active session.
            if (request.isCurrent()) publish(token)
        } finally {
            if (request.isCurrent()) onChecking(false)
        }
    }
    return { restore, login, logout, cancel: requests.cancel }
}
