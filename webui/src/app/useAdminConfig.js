import { useCallback, useEffect, useRef, useState } from 'react'
import { createLatestRequest } from '../utils/latestRequest.js'

const ENV_DRAFT_KEY = 'ds2api_env_config_draft_v1'

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
                try {
                    if (data?.env_backed) localStorage.setItem(ENV_DRAFT_KEY, JSON.stringify(data))
                    else localStorage.removeItem(ENV_DRAFT_KEY)
                } catch { /* Optional draft persistence must not discard a successful fetch. */ }
            }
        } catch (e) {
            if (!request.isCurrent()) return
            console.error('Failed to fetch config:', e)
            showMessage('error', t('errors.fetchConfig', { error: e.message }))
        }
    }, [showMessage, t, token, requests])

    useEffect(() => {
        if (token) {
            try {
                const rawDraft = localStorage.getItem(ENV_DRAFT_KEY)
                if (rawDraft) {
                    const draft = JSON.parse(rawDraft)
                    if (draft?.env_backed) {
                        setConfig(draft)
                    }
                }
            } catch { /* A missing or inaccessible draft is non-fatal. */ }
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
