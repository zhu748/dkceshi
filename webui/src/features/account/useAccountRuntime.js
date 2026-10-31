import { useState } from 'react'

import { parseBindingValue } from './bridgeNodeOptions'

export function useAccountRuntime({ apiFetch, t, onMessage, onRefresh, config, fetchAccounts, onBridgeNodesRefresh }) {
    const [updatingProxy, setUpdatingProxy] = useState({})
    const [togglingEnabled, setTogglingEnabled] = useState({})
    const [togglingAllEnabled, setTogglingAllEnabled] = useState(false)
    const [deletingBanned, setDeletingBanned] = useState(false)
    const [showElasticPool, setShowElasticPool] = useState(false)
    const [elasticPool, setElasticPool] = useState({ enabled: false, per_pool: false, global_count: 3, default_count: 3, no_tools_count: 3, tools_only_count: 3 })
    const [savingElasticPool, setSavingElasticPool] = useState(false)

    // updateAccountBinding 提交「账号代理」下拉的选择。取值由下拉统一编码
    // （见 bridgeNodeOptions.js）：
    //   'auto'        → 交回自动调度（清空绑定 + 清除 no_proxy / manual_binding，
    //                   后端立即按已有测速结果补位分配节点）
    //   ''            → 不走代理（直连，后端置 no_proxy，自动调度不再分配节点）
    //   'node:<key>'  → 绑定代理桥节点（由桥分配本地端口，后端记为手动绑定）
    //   'proxy:<id>'  → 绑定一个手动配置的代理
    const updateAccountBinding = async (identifier, value) => {
        const accountID = String(identifier || '').trim()
        if (!accountID) {
            onMessage('error', t('accountManager.invalidIdentifier'))
            return
        }
        const binding = parseBindingValue(value)
        const endpoint = binding.kind === 'node'
            ? `/admin/mihomo/bindings/${encodeURIComponent(accountID)}`
            : `/admin/accounts/${encodeURIComponent(accountID)}/proxy`
        let payload
        if (binding.kind === 'node') payload = { node: binding.id }
        else if (binding.kind === 'auto') payload = { mode: 'auto' }
        else payload = { proxy_id: binding.kind === 'proxy' ? binding.id : '' }
        setUpdatingProxy(prev => ({ ...prev, [accountID]: true }))
        try {
            const res = await apiFetch(endpoint, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
            })
            const data = await res.json().catch(() => ({}))
            if (!res.ok) {
                onMessage('error', data.detail || t('messages.requestFailed'))
                return
            }
            onMessage('success', t(binding.kind === 'node'
                ? 'accountManager.proxyNodeBindSuccess'
                : binding.kind === 'auto'
                    ? 'accountManager.proxyAutoSuccess'
                    : 'accountManager.proxyUpdateSuccess'))
            await fetchAccounts()
            await onRefresh()
            // 节点上的已绑定账号数变了，刷新下拉里的节点列表。
            await onBridgeNodesRefresh?.()
        } catch (_err) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setUpdatingProxy(prev => ({ ...prev, [accountID]: false }))
        }
    }

    const toggleAccountEnabled = async (identifier, enabled) => {
        const accountID = String(identifier || '').trim()
        if (!accountID) {
            onMessage('error', t('accountManager.invalidIdentifier'))
            return
        }
        if (!enabled && !confirm(t('accountManager.disableAccountConfirm'))) return
        setTogglingEnabled(prev => ({ ...prev, [accountID]: true }))
        try {
            const res = await apiFetch(`/admin/accounts/${encodeURIComponent(accountID)}/enabled`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ enabled }),
            })
            const data = await res.json()
            if (!res.ok) {
                onMessage('error', data.detail || t('messages.requestFailed'))
                return
            }
            onMessage('success', enabled ? t('accountManager.enableAccountSuccess') : t('accountManager.disableAccountSuccess'))
            fetchAccounts()
            onRefresh()
        } catch (_err) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setTogglingEnabled(prev => ({ ...prev, [accountID]: false }))
        }
    }

    const toggleAllAccountsEnabled = async (enabled) => {
        if (!enabled && !confirm(t('accountManager.disableAllAccountsConfirm'))) return
        const allAccounts = config.accounts || []
        if (allAccounts.length === 0) return

        setTogglingAllEnabled(true)
        try {
            const res = await apiFetch('/admin/accounts/enabled/batch', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ enabled }),
            })
            const data = await res.json()
            if (!res.ok) {
                onMessage('error', data.detail || t('messages.requestFailed'))
                return
            }
            onMessage(
                'success',
                enabled
                    ? t('accountManager.enableAllAccountsSuccess', { success: data.total, total: data.total })
                    : t('accountManager.disableAllAccountsSuccess', { success: data.total, total: data.total })
            )
            fetchAccounts()
            onRefresh()
        } catch (_err) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setTogglingAllEnabled(false)
        }
    }

    const deleteBannedAccounts = async () => {
        if (!confirm(t('accountManager.deleteBannedAccountsConfirm'))) return
        setDeletingBanned(true)
        try {
            const res = await apiFetch('/admin/accounts/banned/delete', { method: 'POST' })
            const data = await res.json()
            if (!res.ok) {
                onMessage('error', data.detail || t('messages.requestFailed'))
                return
            }
            onMessage('success', t('accountManager.deleteBannedAccountsSuccess', { total: data.total ?? 0 }))
            fetchAccounts(1)
            onRefresh()
        } catch (_err) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setDeletingBanned(false)
        }
    }

    const openElasticPool = () => {
        const ep = config?.elastic_pool || {}
        setElasticPool({
            enabled: ep.enabled || false,
            per_pool: ep.per_pool || false,
            global_count: ep.global_count ?? 3,
            default_count: ep.default_count ?? 3,
            no_tools_count: ep.no_tools_count ?? 3,
            tools_only_count: ep.tools_only_count ?? 3,
        })
        setShowElasticPool(true)
    }

    const closeElasticPool = () => {
        setShowElasticPool(false)
    }

    const saveElasticPool = async () => {
        const parseCount = (value) => {
            if (value === '') {
                return null
            }
            const n = Number(value)
            if (Number.isNaN(n) || n < 0) {
                return null
            }
            return n
        }

        const gc = parseCount(elasticPool.global_count)
        const dc = parseCount(elasticPool.default_count)
        const nc = parseCount(elasticPool.no_tools_count)
        const tc = parseCount(elasticPool.tools_only_count)

        if (gc === null || dc === null || nc === null || tc === null) {
            return t('accountManager.elasticPoolCountInvalid')
        }

        const payload = {
            enabled: elasticPool.enabled,
            per_pool: elasticPool.per_pool,
            global_count: gc,
            default_count: dc,
            no_tools_count: nc,
            tools_only_count: tc,
        }
        if (!payload.per_pool) {
            payload.default_count = gc
            payload.no_tools_count = gc
            payload.tools_only_count = gc
        }

        setSavingElasticPool(true)
        try {
            const res = await apiFetch('/admin/accounts/elastic-pool', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
            })
            const data = await res.json()
            if (!res.ok) {
                return data.detail || t('messages.requestFailed')
            }
            onMessage('success', t('accountManager.elasticPoolSaveSuccess'))
            await fetchAccounts()
            await onRefresh()
            setShowElasticPool(false)
            return null
        } catch (_err) {
            return t('messages.networkError')
        } finally {
            setSavingElasticPool(false)
        }
    }

    return {
        updatingProxy,
        togglingEnabled,
        togglingAllEnabled,
        deletingBanned,
        showElasticPool,
        elasticPool,
        setElasticPool,
        savingElasticPool,
        updateAccountBinding,
        toggleAccountEnabled,
        toggleAllAccountsEnabled,
        deleteBannedAccounts,
        openElasticPool,
        closeElasticPool,
        saveElasticPool,
    }
}
