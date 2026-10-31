import { useCallback, useEffect, useRef, useState } from 'react'
import { createLatestRequest, createRequestGroup } from '../../utils/latestRequest.js'

// useRealDeviceIds 管理 real 模式下绑定真实设备 id 的账号：拉取清单、按账号刷新、
// 批量刷新。两个动作的区别只在设备档案：
//   刷新 id      —— 复用现有设备档案重新生成：设备身份不变，但数美每次上报都会
//                   换回一个新的 deviceId；
//   重置设备档案  —— 先删除设备档案再生成，等于给该账号换一台新设备。
// 两者都会写回新 id，区别只在设备身份是否保留，因此重置需要二次确认、刷新不需要。
export function useRealDeviceIds({ apiFetch, t, onMessage, onRefresh, fetchAccounts }) {
    const [showRealDeviceIds, setShowRealDeviceIds] = useState(false)
    const [realDeviceIdItems, setRealDeviceIdItems] = useState([])
    const [loadingRealDeviceIds, setLoadingRealDeviceIds] = useState(false)
    const [realDevicePage, setRealDevicePage] = useState(1)
    const [realDevicePageSize, setRealDevicePageSize] = useState(10)
    const [realDeviceTotal, setRealDeviceTotal] = useState(0)
    // realDeviceTotalAll 是过滤前的 real 账号数：批量刷新作用于全量，
    // 界面用它决定批量入口是否可用（搜索筛空不等于没有账号可刷）。
    const [realDeviceTotalAll, setRealDeviceTotalAll] = useState(0)
    const [realDeviceTotalPages, setRealDeviceTotalPages] = useState(1)
    const [realDeviceQuery, setRealDeviceQuery] = useState('')
    const [refreshingRealDeviceId, setRefreshingRealDeviceId] = useState({})
    // bulkRefreshing 为空表示没有批量任务，否则为 'refresh' / 'reset'，用于按钮与行内禁用。
    const [bulkRefreshing, setBulkRefreshing] = useState('')
    // bulkErrors 保存批量刷新的逐账号失败原因，就地显示在对应行下方。
    const [bulkErrors, setBulkErrors] = useState({})
    // 搜索/翻页会并发发出请求；只有最新一次请求才允许提交状态，慢的旧响应
    // 后到时会覆盖新数据（与 useAccountsData 同一防护）。
    const realDeviceRequests = useRef(createLatestRequest()).current
    const mutations = useRef(createRequestGroup()).current
    const cancelRequests = useCallback(() => {
        realDeviceRequests.cancel()
        mutations.cancel()
    }, [realDeviceRequests, mutations])
    useEffect(() => cancelRequests, [cancelRequests])
    const startMutation = key => mutations.has('bulk') ? null : mutations.start(key)
    const currentPage = useRef(null)
    currentPage.current = { page: realDevicePage, pageSize: realDevicePageSize, query: realDeviceQuery }

    // fetchRealDeviceIds 按页拉取。请求页超出范围时（清单被重置/解绑变短、
    // 搜索把结果筛少）自动回落到最后一页，避免停在空页上。
    const fetchRealDeviceIds = useCallback(async (targetPage, targetPageSize = realDevicePageSize, targetQuery = realDeviceQuery) => {
        const request = realDeviceRequests.start()
        setLoadingRealDeviceIds(true)
        try {
            const load = async (page) => {
                let url = `/admin/device-ids/real?page=${page}&page_size=${targetPageSize}`
                const q = String(targetQuery || '').trim()
                if (q) url += `&q=${encodeURIComponent(q)}`
                const res = await apiFetch(url, { signal: request.signal })
                if (!res.ok) {
                    // 5xx 静默失败会让列表一直停留在旧数据上；至少给出提示。
                    if (request.isCurrent()) {
                        onMessage?.('error', t('messages.requestFailed'))
                    }
                    return null
                }
                return await res.json()
            }
            let data = await load(targetPage)
            if (!data || !request.isCurrent()) return
            let page = targetPage
            const totalPages = Math.max(1, Number(data.total_pages) || 1)
            if (page > totalPages) {
                page = totalPages
                data = await load(page)
                if (!data || !request.isCurrent()) return
            }
            if (!request.isCurrent()) return
            setRealDeviceIdItems(Array.isArray(data.items) ? data.items : [])
            setRealDevicePage(page)
            setRealDevicePageSize(targetPageSize)
            setRealDeviceQuery(targetQuery)
            setRealDeviceTotal(Number(data.total) || 0)
            setRealDeviceTotalAll(Number(data.total_all) || 0)
            setRealDeviceTotalPages(Math.max(1, Number(data.total_pages) || 1))
        } catch (e) {
            if (request.isCurrent()) onMessage?.('error', t('messages.networkError'))
        } finally {
            if (request.isCurrent()) setLoadingRealDeviceIds(false)
        }
    }, [apiFetch, onMessage, t, realDevicePageSize, realDeviceQuery])

    const openRealDeviceIds = useCallback(() => {
        setBulkErrors({})
        setShowRealDeviceIds(true)
        fetchRealDeviceIds(1)
    }, [fetchRealDeviceIds])

    const closeRealDeviceIds = useCallback(() => {
        cancelRequests()
        setShowRealDeviceIds(false)
        setLoadingRealDeviceIds(false)
        setBulkRefreshing('')
        setRefreshingRealDeviceId({})
    }, [cancelRequests])

    const changeRealDevicePageSize = useCallback((size) => {
        const next = Number(size) || 10
        setRealDevicePageSize(next)
        fetchRealDeviceIds(1, next)
    }, [fetchRealDeviceIds])

    // searchRealDeviceIds 与账号列表一致：每次输入都直接查（服务端过滤 + 回到第一页）。
    const searchRealDeviceIds = useCallback((query) => {
        setRealDeviceQuery(query)
        fetchRealDeviceIds(1, realDevicePageSize, query)
    }, [fetchRealDeviceIds, realDevicePageSize])

    const prevRealDevicePage = useCallback(() => {
        fetchRealDeviceIds(Math.max(1, realDevicePage - 1))
    }, [fetchRealDeviceIds, realDevicePage])

    const nextRealDevicePage = useCallback(() => {
        fetchRealDeviceIds(Math.min(realDeviceTotalPages, realDevicePage + 1))
    }, [fetchRealDeviceIds, realDevicePage, realDeviceTotalPages])

    // afterRefresh 在任一次刷新成功后统一收口：列表（当前页与当前搜索）、账号、
    // 上层统计都要跟着变。
    const afterRefresh = useCallback(async (request) => {
        const current = currentPage.current
        await fetchRealDeviceIds(current.page, current.pageSize, current.query)
        if (!request.isCurrent()) return
        await fetchAccounts?.()
        if (request.isCurrent()) onRefresh?.()
    }, [fetchAccounts, fetchRealDeviceIds, onRefresh, realDevicePage, realDevicePageSize, realDeviceQuery])

    const refreshRealDeviceId = useCallback(async (identifier, resetProfile) => {
        if (resetProfile && !confirm(t('accountManager.realDeviceIdResetConfirm', { account: identifier }))) {
            return
        }
        const request = startMutation(identifier)
        if (!request) return
        setRefreshingRealDeviceId(prev => ({ ...prev, [identifier]: true }))
        setBulkErrors(prev => {
            if (!prev[identifier]) return prev
            const next = { ...prev }
            delete next[identifier]
            return next
        })
        try {
            const res = await apiFetch('/admin/device-ids/real/refresh', {
                method: 'POST', signal: request.signal,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ identifier, reset_profile: Boolean(resetProfile) }),
            })
            const data = await res.json().catch(() => ({}))
            if (!request.isCurrent()) return
            if (!res.ok) {
                onMessage('error', data.detail || t('messages.requestFailed'))
                return
            }
            onMessage('success', t(
                resetProfile
                    ? 'accountManager.realDeviceIdResetSuccess'
                    : 'accountManager.realDeviceIdRefreshSuccess',
                { account: identifier },
            ))
            await afterRefresh(request)
        } catch (_err) {
            if (request.isCurrent()) onMessage('error', t('messages.networkError'))
        } finally {
            if (request.isCurrent()) setRefreshingRealDeviceId(prev => ({ ...prev, [identifier]: false }))
            request.finish()
        }
    }, [afterRefresh, apiFetch, onMessage, t])

    const refreshAllRealDeviceIds = useCallback(async (resetProfile) => {
        if (resetProfile && !confirm(t('accountManager.realDeviceIdResetAllConfirm'))) {
            return
        }
        if (mutations.size) return
        const request = startMutation('bulk')
        if (!request) return
        setBulkRefreshing(resetProfile ? 'reset' : 'refresh')
        setBulkErrors({})
        try {
            const res = await apiFetch('/admin/device-ids/real/refresh-all', {
                method: 'POST', signal: request.signal,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ reset_profile: Boolean(resetProfile) }),
            })
            const data = await res.json().catch(() => ({}))
            if (!request.isCurrent()) return
            if (!res.ok) {
                onMessage('error', data.detail || t('messages.requestFailed'))
                return
            }
            const refreshed = Number(data.refreshed || 0)
            const failed = Number(data.failed || 0)
            if (failed > 0) {
                const failures = {}
                for (const item of Array.isArray(data.items) ? data.items : []) {
                    if (item?.error) failures[item.identifier] = item.error
                }
                setBulkErrors(failures)
                onMessage('error', t('accountManager.realDeviceIdBulkFailed', { refreshed, failed }))
            } else {
                onMessage('success', t(
                    resetProfile
                        ? 'accountManager.realDeviceIdResetAllSuccess'
                        : 'accountManager.realDeviceIdRefreshAllSuccess',
                    { count: refreshed },
                ))
            }
            await afterRefresh(request)
        } catch (_err) {
            if (request.isCurrent()) onMessage('error', t('messages.networkError'))
        } finally {
            if (request.isCurrent()) setBulkRefreshing('')
            request.finish()
        }
    }, [afterRefresh, apiFetch, onMessage, t])

    return {
        showRealDeviceIds,
        openRealDeviceIds,
        closeRealDeviceIds,
        realDeviceIdItems,
        loadingRealDeviceIds,
        realDevicePage,
        realDevicePageSize,
        realDeviceTotal,
        realDeviceTotalAll,
        realDeviceTotalPages,
        realDeviceQuery,
        changeRealDevicePageSize,
        prevRealDevicePage,
        nextRealDevicePage,
        searchRealDeviceIds,
        refreshingRealDeviceId,
        bulkRefreshing,
        bulkErrors,
        fetchRealDeviceIds,
        refreshRealDeviceId,
        refreshAllRealDeviceIds,
    }
}
