import { useCallback, useEffect, useRef, useState } from 'react'
import { clearStoredConfigDraft } from './adminSession.js'
import { createLatestRequest } from '../utils/latestRequest.js'

export function useAdminConfig({ token, showMessage, t }) {
    const [config, setConfig] = useState({ keys: [], accounts: [] })
    const requests = useRef(createLatestRequest()).current

    const fetchConfig = useCallback(async () => {
        if (!token) return
        const request = requests.start()
        try {
            const res = await fetch('/admin/config', {
                headers: { 'Authorization': `Bearer ${token}` }, signal: request.signal,
            })
            if (res.ok) {
                const data = await res.json()
                if (!request.isCurrent()) return
                setConfig(data)
            }
        } catch (e) {
            if (!request.isCurrent()) return
            console.error('Failed to fetch config:', e)
            showMessage('error', t('errors.fetchConfig', { error: e.message }))
        }
    }, [showMessage, t, token, requests])

    useEffect(() => {
        clearStoredConfigDraft()
        if (token) {
            fetchConfig()
        } else {
            setConfig({ keys: [], accounts: [] })
        }
        return () => requests.cancel()
    }, [fetchConfig, token, requests])

    return {
        config,
        fetchConfig,
    }
}
