import { useMemo, useState } from 'react'
import { useI18n } from '../../i18n'
import { accountDisplay, groupUsage } from './usageDashboard'
import { fmtMoney, fmtNumber } from './usageUtils'
import './breakdown.css'

const PAGE_SIZE = 10

export default function UsageBreakdown({ records, onAccount, onModel, filterKey = '' }) {
    const { lang, t } = useI18n()
    const [dimension, setDimension] = useState('account')
    const [sort, setSort] = useState('total')
    const [pagination, setPagination] = useState({ key: '', page: 0 })
    const queryKey = JSON.stringify([filterKey, dimension, sort])
    const page = pagination.key === queryKey ? pagination.page : 0
    const setPage = (next) => setPagination({ key: queryKey, page: next })
    const rows = useMemo(() => groupUsage(records, dimension).sort((a, b) => b[sort] - a[sort] || a.id.localeCompare(b.id)), [records, dimension, sort])
    const lastPage = Math.max(0, Math.ceil(rows.length / PAGE_SIZE) - 1)
    const currentPage = Math.min(page, lastPage)
    const shown = rows.slice(currentPage * PAGE_SIZE, (currentPage + 1) * PAGE_SIZE)
    const fmt = (value) => fmtNumber(value, lang)
    const dateFormatter = useMemo(() => new Intl.DateTimeFormat(lang === 'zh' ? 'zh-CN' : 'en-US', {
        timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit',
        hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
    }), [lang])
    const percentFormatter = useMemo(() => new Intl.NumberFormat(lang === 'zh' ? 'zh-CN' : 'en-US', {
        style: 'percent', maximumFractionDigits: 1,
    }), [lang])

    return (
        <section className="usage-breakdown" aria-label={t('usage.breakdown')}>
            <div className="usage-breakdown-head">
                <div className="usage-segmented" role="group" aria-label={t('usage.breakdown')}>
                    {['account', 'model'].map((value) => (
                        <button key={value} type="button" aria-pressed={dimension === value}
                            className={`usage-seg-btn${dimension === value ? ' active' : ''}`}
                            onClick={() => { setDimension(value) }}>
                            {t(value === 'account' ? 'usage.accountUsage' : 'usage.modelUsage')}
                        </button>
                    ))}
                </div>
                <label className="usage-breakdown-sort">
                    {t('usage.sortBy')}
                    <select value={sort} onChange={(e) => { setSort(e.target.value) }}>
                        {['total', 'calls', 'cost', 'lastUsedAt'].map((key) => (
                            <option key={key} value={key}>{t(`usage.sort.${key}`)}</option>
                        ))}
                    </select>
                </label>
            </div>
            <p className="usage-breakdown-note">{t('usage.accountingNote')}</p>
            <div className="usage-table-scroll" tabIndex={0} role="region" aria-label={t('usage.breakdown')}>
                <table className="usage-table">
                    <caption className="usage-table-caption">{t(dimension === 'account' ? 'usage.accountUsage' : 'usage.modelUsage')}</caption>
                    <thead><tr>
                        <th scope="col">{t(`usage.${dimension}`)}</th>
                        <th scope="col">{t('usage.apiRequests')}</th>
                        <th scope="col">{t('usage.inputTokens')}</th>
                        <th scope="col">{t('usage.outputTokens')}</th>
                        <th scope="col">{t('usage.reasoningTokens')}</th>
                        <th scope="col">{t('usage.totalTokens')}</th>
                        <th scope="col">{t('usage.tokenShare')}</th>
                        <th scope="col">{t('usage.estimatedCost')}</th>
                        <th scope="col">{t('usage.lastUsedAt')}</th>
                    </tr></thead>
                    <tbody>
                        {shown.map((row) => (
                            <tr key={row.id}>
                                <th scope="row"><button type="button" className="usage-row-link" title={dimension === 'account' ? `${accountDisplay(row.id, row.label, t)} (${row.id})` : row.id}
                                    onClick={() => (dimension === 'account' ? onAccount : onModel)(row.id)}>
                                    {dimension === 'account' ? accountDisplay(row.id, row.label, t) : row.id}
                                </button></th>
                                <td>{fmt(row.calls)}</td><td>{fmt(row.prompt)}</td><td>{fmt(row.output)}</td>
                                <td>{fmt(row.reasoning)}</td><td>{fmt(row.total)}</td>
                                <td>{percentFormatter.format(row.share)}</td><td>{fmtMoney(row.cost, lang, row.cost > 0 && row.cost < 0.01 ? 8 : 2)}</td>
                                <td className="usage-last-used">{row.lastUsedAt > 0 ? dateFormatter.format(row.lastUsedAt) : '—'}</td>
                            </tr>
                        ))}
                        {shown.length === 0 && <tr><td colSpan={9} className="usage-table-empty">{t('usage.noMatchingUsage')}</td></tr>}
                    </tbody>
                </table>
            </div>
            <div className="usage-breakdown-footer">
                <span>{t('usage.breakdownCount', { count: String(rows.length) })}</span>
                <div>
                    <button type="button" className="usage-btn" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>{t('usage.previousPage')}</button>
                    <span className="usage-page-number">{currentPage + 1} / {lastPage + 1}</span>
                    <button type="button" className="usage-btn" disabled={currentPage >= lastPage} onClick={() => setPage(currentPage + 1)}>{t('usage.nextPage')}</button>
                </div>
            </div>
        </section>
    )
}
