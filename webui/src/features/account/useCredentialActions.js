import { useState } from 'react'

import { useManualTokenActions } from './useManualTokenActions.js'
import { credentialFlagFormValues, credentialFlagPayload } from './credentialFlags'

export function useCredentialActions(options) {
    const { apiFetch, t, onMessage, onRefresh, config } = options
    const manualToken = useManualTokenActions(options)
    const [showAddKey, setShowAddKey] = useState(false)
    const [editingKey, setEditingKey] = useState(null)
    const [newKey, setNewKey] = useState({ key: '', name: '', remark: '', flags: {} })
    const [copiedKey, setCopiedKey] = useState(null)
    const [loading, setLoading] = useState(false)

    const openAddKey = () => {
        setEditingKey(null)
        setNewKey({ key: '', name: '', remark: '', flags: credentialFlagFormValues(config, null) })
        setShowAddKey(true)
    }

    const openEditKey = (item) => {
        if (!item?.key) return
        setEditingKey(item)
        setNewKey({
            key: item.key || '',
            name: item.name || '',
            remark: item.remark || '',
            flags: credentialFlagFormValues(config, item),
        })
        setShowAddKey(true)
    }

    const closeKeyModal = () => {
        setShowAddKey(false)
        setEditingKey(null)
        setNewKey({ key: '', name: '', remark: '', flags: {} })
    }

    const addKey = async () => {
        const isEditing = Boolean(editingKey?.key)
        if (!isEditing && !newKey.key.trim()) {
            return
        }
        setLoading(true)
        try {
            const endpoint = isEditing
                ? `/admin/keys/${encodeURIComponent(editingKey.key)}`
                : '/admin/keys'
            const method = isEditing ? 'PUT' : 'POST'
            const flags = credentialFlagPayload(config, newKey.flags)
            const payload = isEditing
                ? { name: newKey.name, remark: newKey.remark, ...flags }
                : { key: newKey.key.trim(), name: newKey.name, remark: newKey.remark, ...flags }
            if (!isEditing && !payload.key) {
                return
            }
            const res = await apiFetch(endpoint, {
                method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
            })
            if (res.ok) {
                onMessage('success', isEditing ? t('accountManager.updateKeySuccess') : t('accountManager.addKeySuccess'))
                closeKeyModal()
                onRefresh()
            } else {
                const data = await res.json()
                onMessage('error', data.detail || (isEditing ? t('messages.requestFailed') : t('messages.failedToAdd')))
            }
        } catch (e) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setLoading(false)
        }
    }

    const deleteKey = async (key) => {
        if (!confirm(t('accountManager.deleteKeyConfirm'))) return
        try {
            const res = await apiFetch(`/admin/keys/${encodeURIComponent(key)}`, { method: 'DELETE' })
            if (res.ok) {
                onMessage('success', t('messages.deleted'))
                onRefresh()
            } else {
                onMessage('error', t('messages.deleteFailed'))
            }
        } catch (e) {
            onMessage('error', t('messages.networkError'))
        }
    }

    return {
        ...manualToken,
        showAddKey,
        editingKey,
        newKey,
        setNewKey,
        copiedKey,
        setCopiedKey,
        loading,
        openAddKey,
        openEditKey,
        closeKeyModal,
        addKey,
        deleteKey,
    }
}
