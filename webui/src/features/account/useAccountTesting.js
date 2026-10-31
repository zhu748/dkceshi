import { useEffect, useRef, useState } from 'react'

import { createRequestGroup } from '../../utils/latestRequest.js'
import { createBatchAccountTests } from './batchAccountTests.js'

export function useAccountTesting({ apiFetch, t, onMessage, onRefresh, config, fetchAccounts, resolveAccountIdentifier, offerManualToken }) {
    const [testing, setTesting] = useState({})
    const [testingAll, setTestingAll] = useState(false)
    const [batchProgress, setBatchProgress] = useState({ current: 0, total: 0, results: [] })
    const [sessionCounts, setSessionCounts] = useState({})
    const [deletingSessions, setDeletingSessions] = useState({})

    const requests = useRef(createRequestGroup()).current
    useEffect(() => () => requests.cancel(), [requests])
    const batch = useRef(null)
    if (!batch.current) {
        batch.current = createBatchAccountTests({ onRunning: setTestingAll, onProgress: setBatchProgress })
    }
    useEffect(() => () => batch.current.dispose(), [])

    const testAccount = async (identifier) => {
        const accountID = String(identifier || '').trim()
        if (!accountID) {
            onMessage('error', t('accountManager.invalidIdentifier'))
            return
        }
        const request = requests.start('test:' + accountID)
        if (!request) return
        setTesting(prev => ({ ...prev, [accountID]: true }))
        try {
            const res = await apiFetch('/admin/accounts/test', {
                method: 'POST', signal: request.signal,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ identifier: accountID }),
            })
            const data = await res.json()
            if (!request.isCurrent()) return
            
            // 更新会话数
            if (data.session_count !== undefined) {
                setSessionCounts(prev => ({ ...prev, [accountID]: data.session_count }))
            }

            const isMissingLoginToken = !data.success && typeof data.message === 'string' && (
                data.message.includes('missing login token: login rejected by upstream') ||
                data.message.includes('missing login token')
            )
            if (isMissingLoginToken) {
                offerManualToken({
                    identifier: accountID,
                    username: data.account_username || accountID,
                    password: data.account_password || '',
                })
            }
            
            const statusMessage = data.success
                ? t('apiTester.testSuccess', { account: accountID, time: data.response_time })
                : `${accountID}: ${data.message}`
            onMessage(data.success ? 'success' : 'error', statusMessage)
            fetchAccounts()
            onRefresh()
        } catch (e) {
            if (request.isCurrent()) onMessage('error', t('accountManager.testFailed', { error: e.message }))
        } finally {
            if (request.isCurrent()) setTesting(prev => ({ ...prev, [accountID]: false }))
            request.finish()
        }
    }

    const testAllAccounts = async () => {
        if (batch.current.running || !confirm(t('accountManager.testAllConfirm'))) return
        try {
            await batch.current.run({
                accounts: config.accounts || [], apiFetch, resolveAccountIdentifier,
                invalidIdentifier: t('accountManager.invalidIdentifier'),
                onCompleted: ({ success, total }) => {
                    onMessage('success', t('accountManager.testAllCompleted', { success, total }))
                    fetchAccounts()
                    onRefresh()
                },
            })
        } catch (error) {
            onMessage('error', t('accountManager.testFailed', { error: error.message }))
        }
    }

    const cancelAccountTests = () => {
        const progress = batch.current.cancel()
        if (progress) {
            setBatchProgress({ ...progress, stopped: true })
            onMessage('warning', t('accountManager.testAllStopped', progress))
        }
    }

    const deleteAllSessions = async (identifier) => {
        const accountID = String(identifier || '').trim()
        if (!accountID) {
            onMessage('error', t('accountManager.invalidIdentifier'))
            return
        }
        if (!confirm(t('accountManager.deleteAllSessionsConfirm'))) return
        
        const request = requests.start('sessions:' + accountID)
        if (!request) return
        setDeletingSessions(prev => ({ ...prev, [accountID]: true }))
        try {
            const res = await apiFetch('/admin/accounts/sessions/delete-all', {
                method: 'POST', signal: request.signal,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ identifier: accountID }),
            })
            const data = await res.json()
            if (!request.isCurrent()) return
            
            if (data.success) {
                onMessage('success', t('accountManager.deleteAllSessionsSuccess'))
                // 清除会话数显示
                setSessionCounts(prev => {
                    const newCounts = { ...prev }
                    delete newCounts[accountID]
                    return newCounts
                })
            } else {
                onMessage('error', data.message || t('messages.requestFailed'))
            }
        } catch (e) {
            if (request.isCurrent()) onMessage('error', t('messages.networkError'))
        } finally {
            if (request.isCurrent()) setDeletingSessions(prev => ({ ...prev, [accountID]: false }))
            request.finish()
        }
    }

    return {
        testing,
        testingAll,
        batchProgress,
        sessionCounts,
        deletingSessions,
        testAccount,
        testAllAccounts,
        cancelAccountTests,
        deleteAllSessions,
    }
}
