import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { detectRuntimeEnv } from '../utils/runtimeEnv'
import { createAdminSession } from './adminSession.js'

export function useAdminAuth({ isProduction, location }) {
    const [message, setMessage] = useState(null)
    const [token, setToken] = useState(null)
    const [authChecking, setAuthChecking] = useState(true)
    const messageTimer = useRef(null)

    const isAdminRoute = location.pathname.startsWith('/admin') || isProduction
    const runtimeEnv = useMemo(() => detectRuntimeEnv(), [])
    const isVercel = runtimeEnv.isVercel
    const session = useMemo(() => createAdminSession({ onToken: setToken, onChecking: setAuthChecking }), [])

    const showMessage = useCallback((type, text) => {
        clearTimeout(messageTimer.current)
        setMessage({ type, text })
        messageTimer.current = setTimeout(() => {
            setMessage(null)
            messageTimer.current = null
        }, 5000)
    }, [])

    useEffect(() => () => clearTimeout(messageTimer.current), [])

    useEffect(() => {
        if (!isAdminRoute) {
            setAuthChecking(false)
            return undefined
        }
        session.restore()
        return () => session.cancel()
    }, [isAdminRoute, session])

    return {
        token,
        authChecking,
        message,
        isAdminRoute,
        isVercel,
        showMessage,
        handleLogin: session.login,
        handleLogout: session.logout,
    }
}
