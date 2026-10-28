import { useEffect, useState } from 'react'

export function useAccountsData({ apiFetch, onConfigRefresh }) {
    const [queueStatus, setQueueStatus] = useState(null)
    const [keysExpanded, setKeysExpanded] = useState(false)

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
        setLoadingAccounts(true)
        try {
            let url = `/admin/accounts?page=${targetPage}&page_size=${targetPageSize}`
            if (targetQuery.trim()) url += `&q=${encodeURIComponent(targetQuery.trim())}`
            if (targetStatus && targetStatus !== 'all') url += `&status=${encodeURIComponent(targetStatus)}`
            const res = await apiFetch(url)
            if (res.ok) {
                const data = await res.json()
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
                // 账号列表是最新的（自动调度可能在后台重绑了账号），
                // 同步刷新 config 让 proxies（含托管代理名）不过期，
                // 避免账号显示成 mihomo-<hash> 的原始代理 ID。
                onConfigRefresh?.()
            }
        } catch (e) {
            console.error('Failed to fetch accounts:', e)
        } finally {
            setLoadingAccounts(false)
        }
    }

    const changePageSize = (newSize) => {
        setPageSize(newSize)
        fetchAccounts(1, newSize, searchQuery, filterStatus)
    }

    const handleSearchChange = (query) => {
        setSearchQuery(query)
        fetchAccounts(1, pageSize, query, filterStatus)
    }

    const handleFilterChange = (status) => {
        setFilterStatus(status)
        fetchAccounts(1, pageSize, searchQuery, status)
    }

    const fetchQueueStatus = async () => {
        try {
            const res = await apiFetch('/admin/queue/status')
            if (res.ok) {
                const data = await res.json()
                setQueueStatus(data)
            }
        } catch (e) {
            console.error('Failed to fetch queue status:', e)
        }
    }

    useEffect(() => {
        fetchAccounts()
        fetchQueueStatus()
        const interval = setInterval(fetchQueueStatus, 5000)
        return () => clearInterval(interval)
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
