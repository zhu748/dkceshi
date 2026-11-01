import { useEffect, useRef, useState } from 'react'
import { createLatestRequest } from '../../utils/latestRequest.js'

export function useAccountsData({ apiFetch, t, onConfigRefresh, onMessage }) {
    const [queueStatus, setQueueStatus] = useState(null)
    const [keysExpanded, setKeysExpanded] = useState(false)
    const accountRequests = useRef(createLatestRequest()).current
    const queueRequests = useRef(createLatestRequest()).current
    const searchTimer = useRef(null)
    const lastConfigRefresh = useRef(0)

    const [accounts, setAccounts] = useState([])
    const [page, setPage] = useState(1)
    const [pageSize, setPageSize] = useState(10)
    const [totalPages, setTotalPages] = useState(1)
    const [totalAccounts, setTotalAccounts] = useState(0)
    const [loadingAccounts, setLoadingAccounts] = useState(false)

    const resolveAccountIdentifier = (acc) => {
        if (!acc || typeof acc !== 'object') return ''
        return String(acc.identifier || acc.email || acc.mobile || '').trim()
    }

    const [searchQuery, setSearchQuery] = useState('')
    const [filterStatus, setFilterStatus] = useState('all')
    const [accountCounts, setAccountCounts] = useState({
        total: 0,
        enabled: 0,
        disabled: 0,
        muted: 0,
        banned: 0,
    })

    const fetchAccounts = async (
        targetPage = page,
        targetPageSize = pageSize,
        targetQuery = searchQuery,
        targetStatus = filterStatus
    ) => {
        clearTimeout(searchTimer.current)
        const request = accountRequests.start()
        setLoadingAccounts(true)
        try {
            let url = `/admin/accounts?page=${targetPage}&page_size=${targetPageSize}`
            if (targetQuery.trim()) url += `&q=${encodeURIComponent(targetQuery.trim())}`
            if (targetStatus && targetStatus !== 'all') url += `&status=${encodeURIComponent(targetStatus)}`
            const res = await apiFetch(url, { signal: request.signal })
            if (res.ok) {
                const data = await res.json()
                if (!request.isCurrent()) return
                const nextCounts = data.counts && typeof data.counts === 'object'
                    ? {
                        total: data.counts.total ?? 0,
                        enabled: data.counts.enabled ?? 0,
                        disabled: data.counts.disabled ?? 0,
                        muted: data.counts.muted ?? 0,
                        banned: data.counts.banned ?? 0,
                    }
                    : null
                if (nextCounts) {
                    setAccountCounts(nextCounts)
                    if (
                        (targetStatus === 'muted' && nextCounts.muted === 0) ||
                        (targetStatus === 'banned' && nextCounts.banned === 0)
                    ) {
                        setFilterStatus('all')
                        return await fetchAccounts(1, targetPageSize, targetQuery, 'all')
                    }
                }
                setAccounts(data.items || [])
                setTotalPages(data.total_pages || 1)
                setTotalAccounts(data.total || 0)
                setPage(data.page || 1)
                // Coalesce full-config refreshes while paging/searching.
                if (Date.now() - lastConfigRefresh.current >= 5000) {
                    lastConfigRefresh.current = Date.now()
                    onConfigRefresh?.()
                }
            } else if (request.isCurrent()) {
                // 非 200 静默失败会让列表停留在旧数据上；至少给出提示。
                onMessage?.('error', t('messages.requestFailed'))
            }
        } catch (e) {
            if (!request.isCurrent()) return
            console.error('Failed to fetch accounts:', e)
        } finally {
            if (request.isCurrent()) setLoadingAccounts(false)
        }
    }

    const changePageSize = (newSize) => {
        setPageSize(newSize)
        fetchAccounts(1, newSize, searchQuery, filterStatus)
    }

    const handleSearchChange = (query) => {
        setSearchQuery(query)
        accountRequests.cancel()
        clearTimeout(searchTimer.current)
        searchTimer.current = setTimeout(() => fetchAccounts(1, pageSize, query, filterStatus), 250)
    }

    const handleFilterChange = (status) => {
        setFilterStatus(status)
        fetchAccounts(1, pageSize, searchQuery, status)
    }

    const fetchQueueStatus = async () => {
        const request = queueRequests.start()
        try {
            const res = await apiFetch('/admin/queue/status', { signal: request.signal })
            if (res.ok) {
                const data = await res.json()
                if (!request.isCurrent()) return
                setQueueStatus(data)
            }
        } catch (e) {
            if (!request.isCurrent()) return
            console.error('Failed to fetch queue status:', e)
        }
    }

    useEffect(() => {
        fetchAccounts()
        fetchQueueStatus()
        const interval = setInterval(fetchQueueStatus, 5000)
        return () => {
            clearInterval(interval)
            clearTimeout(searchTimer.current)
            accountRequests.cancel()
            queueRequests.cancel()
        }
    }, [])

    return {
        queueStatus,
        keysExpanded,
        setKeysExpanded,
        accounts,
        page,
        pageSize,
        totalPages,
        totalAccounts,
        loadingAccounts,
        fetchAccounts,
        changePageSize,
        resolveAccountIdentifier,
        searchQuery,
        handleSearchChange,
        filterStatus,
        handleFilterChange,
        accountCounts,
    }
}
