import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useI18n } from '../../i18n'
import { sameUsageRecords } from './usageUtils.js'
import { createUsageRecordsResource } from './usageRecordsResource.js'

export function useUsageRecords({ authFetch, onMessage }) {
    const { t } = useI18n()
    const [state, setState] = useState({ records: [], loading: true, initialized: false, error: '' })
    const tRef = useRef(t)
    const onMessageRef = useRef(onMessage)
    useEffect(() => {
        tRef.current = t
        onMessageRef.current = onMessage
    })

    const resource = useMemo(() => createUsageRecordsResource({
        onChange: (next) => setState((prev) => ({
            ...prev, ...next,
            records: !next.records || sameUsageRecords(prev.records, next.records) ? prev.records : next.records,
        })),
        onError: (message) => onMessageRef.current?.('error', message),
    }), [authFetch])
    const loadRecords = useCallback(() => resource.load(authFetch, tRef.current('usage.loadFailed')), [resource, authFetch])
    const clearRecords = useCallback(() => resource.clear(), [resource])
    useEffect(() => {
        resource.activate()
        loadRecords()
        return () => resource.dispose()
    }, [resource, loadRecords])

    return { ...state, loadRecords, clearRecords }
}
