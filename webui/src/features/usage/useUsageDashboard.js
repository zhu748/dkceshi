import { useMemo } from 'react'
import { dayKey } from './usageUtils.js'
import { aggregateDaily, fillDateRange } from './usageAggregation.js'
import { accountOptionsFor, buildModelUsageRows, filterUsageRecords, usageTotals } from './usageDashboard.js'

// Keep data derivation separate from chart presentation and mutation actions.
export function useUsageDashboard({ records, dateRange, modelFilter, accountFilter, t }) {
    const today = dayKey(Date.now())
    const filtered = useMemo(() => filterUsageRecords(records, {
        dateRange, model: modelFilter, account: accountFilter,
    }), [records, dateRange, modelFilter, accountFilter, today])
    const accountOptions = useMemo(() => accountOptionsFor(records, t), [records, t])

    const saved = useMemo(() => usageTotals(records), [records])
    const summary = useMemo(() => {
        const windowTotals = usageTotals(filtered)
        return {
            savedTotal: saved.total, savedCost: saved.cost,
            windowTotal: windowTotals.total, windowCost: windowTotals.cost,
            promptSum: windowTotals.prompt, completionSum: windowTotals.output, calls: windowTotals.calls,
        }
    }, [saved, filtered])

    const dailyAll = useMemo(() => fillDateRange(aggregateDaily(filtered), dateRange), [filtered, dateRange])

    const modelOptions = useMemo(() => {
        const set = new Set()
        records.forEach((r) => set.add(r.model || 'unknown'))
        return Array.from(set).sort()
    }, [records])

    const modelRows = useMemo(() => buildModelUsageRows(filtered, dateRange), [filtered, dateRange])
    return { filtered, accountOptions, summary, dailyAll, modelOptions, modelRows }
}
