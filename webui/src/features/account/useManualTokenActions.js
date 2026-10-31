import { useEffect, useRef, useState } from 'react'

import { createManualTokenResource } from './manualTokenResource.js'

export function useManualTokenActions({ apiFetch, t, onMessage, fetchAccounts, onRefresh, hideAccountEditor }) {
    const [state, setState] = useState({ open: false, account: null, token: '', saving: false })
    const resource = useRef(null)
    if (!resource.current) {
        resource.current = createManualTokenResource(patch => setState(prev => ({ ...prev, ...patch })))
    }
    useEffect(() => () => resource.current.cancel(), [])

    const openManualTokenModal = (identifier) => {
        const accountID = String(identifier || '').trim()
        if (!accountID) {
            onMessage('error', t('accountManager.invalidIdentifier'))
            return
        }
        hideAccountEditor()
        return resource.current.open(accountID, apiFetch)
    }
    const submitManualToken = () => resource.current.submit(state.token, apiFetch, {
        onRequired: () => onMessage('error', t('accountManager.tokenRequired')),
        onRejected: detail => onMessage('error', detail || t('messages.requestFailed')),
        onError: () => onMessage('error', t('messages.networkError')),
        onSaved: () => {
            onMessage('success', t('accountManager.manualTokenSuccess'))
            fetchAccounts()
            onRefresh()
        },
    })

    return {
        showManualToken: state.open,
        manualTokenAccount: state.account,
        manualToken: state.token,
        setManualToken: value => setState(prev => ({ ...prev, token: typeof value === 'function' ? value(prev.token) : value })),
        submittingManualToken: state.saving,
        openManualTokenModal,
        closeManualTokenModal: resource.current.close,
        submitManualToken,
        offerManualToken: resource.current.offer,
    }
}
