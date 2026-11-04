import { useCallback, useEffect, useRef, useState } from 'react'

import { createStatusPoll } from './statusPoll.js'

export function useVercelSyncState({ apiFetch, onMessage, t, isVercel = false }) {
    const [vercelToken, setVercelToken] = useState('')
    const [projectId, setProjectId] = useState('')
    const [teamId, setTeamId] = useState('')
    const [saveCredentials, setSaveCredentials] = useState(true)
    const [loading, setLoading] = useState(false)
    const [result, setResult] = useState(null)
    const [preconfig, setPreconfig] = useState(null)
    const [syncStatus, setSyncStatus] = useState(null)
    const [pollPaused, setPollPaused] = useState(false)
    const [pollFailures, setPollFailures] = useState(0)
    const poll = useRef(null)
    const ui = useRef({ onMessage, t })
    ui.current = { onMessage, t }
    const fetchSyncStatus = useCallback(options => poll.current?.refresh(options), [])

    useEffect(() => {
        const controller = new AbortController()
        apiFetch('/admin/vercel/config', { signal: controller.signal }).then(async res => {
            if (!res.ok) return
            const data = await res.json()
            if (controller.signal.aborted) return
            setPreconfig(data)
            if (data.project_id) setProjectId(data.project_id)
            if (data.team_id) setTeamId(data.team_id)
        }).catch(error => { if (!controller.signal.aborted) console.error('Failed to load preconfig:', error) })
        const resource = createStatusPoll({
            isVercel,
            load: async signal => {
                const res = await apiFetch('/admin/vercel/status', { signal })
                if (!res.ok) throw new Error(`status ${res.status}`)
                return res.json()
            },
            onState: state => {
                if ('syncStatus' in state) setSyncStatus(state.syncStatus)
                setPollFailures(state.pollFailures)
                setPollPaused(state.pollPaused)
            },
            onError: () => ui.current.onMessage('error', ui.current.t('vercel.networkError')),
        })
        poll.current = resource
        resource.start()
        return () => { controller.abort(); resource.stop(); if (poll.current === resource) poll.current = null }
    }, [apiFetch, isVercel])

    const handleManualRefresh = useCallback(() => fetchSyncStatus({ manual: true }), [fetchSyncStatus])

    const handleSync = useCallback(async () => {
        const tokenToUse = preconfig?.has_token && !vercelToken ? '__USE_PRECONFIG__' : vercelToken

        if (!tokenToUse && !preconfig?.has_token) {
            onMessage('error', t('vercel.tokenRequired'))
            return
        }
        if (!projectId) {
            onMessage('error', t('vercel.projectRequired'))
            return
        }

        setLoading(true)
        setResult(null)
        try {
            const res = await apiFetch('/admin/vercel/sync', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    vercel_token: tokenToUse,
                    project_id: projectId,
                    team_id: teamId || undefined,
                    save_credentials: saveCredentials,
                }),
            })
            const data = await res.json()
            if (res.ok) {
                setResult({ ...data, success: true })
                onMessage('success', data.message)
                fetchSyncStatus()
            } else {
                setResult({ ...data, success: false })
                onMessage('error', data.detail || t('vercel.syncFailed'))
            }
        } catch (_e) {
            onMessage('error', t('vercel.networkError'))
        } finally {
            setLoading(false)
        }
    }, [apiFetch, fetchSyncStatus, onMessage, preconfig?.has_token, projectId, saveCredentials, t, teamId, vercelToken])

    return {
        vercelToken,
        setVercelToken,
        projectId,
        setProjectId,
        teamId,
        setTeamId,
        saveCredentials,
        setSaveCredentials,
        loading,
        result,
        preconfig,
        syncStatus,
        pollPaused,
        pollFailures,
        handleManualRefresh,
        handleSync,
    }
}
