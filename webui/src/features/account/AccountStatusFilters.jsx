import clsx from 'clsx'

const filters = [
    ['all', 'filterAll', 'total'], ['enabled', 'filterEnabled', 'enabled'], ['disabled', 'filterDisabled', 'disabled'],
    ['cooling_down', 'filterCoolingDown', 'cooling_down'], ['muted', 'filterMuted', 'muted'], ['banned', 'filterBanned', 'banned'],
]

export default function AccountStatusFilters({ counts, filterStatus, onFilterChange, t }) {
    return <div className="px-6 py-3 border-b border-border bg-muted/20 flex items-center justify-end gap-1.5 flex-wrap">
        {filters.filter(([status, , key]) => ['all', 'enabled', 'disabled'].includes(status) || counts[key] > 0).map(([status, label, key]) => (
            <button key={status} onClick={() => onFilterChange?.(status)} className={clsx(
                'px-2.5 py-1.5 rounded-lg text-xs font-medium transition-colors border',
                filterStatus === status ? 'bg-primary text-primary-foreground border-primary'
                    : status === 'cooling_down' ? 'bg-card text-amber-500 border-border hover:bg-amber-500/10'
                        : ['muted', 'banned'].includes(status) ? 'bg-card text-destructive border-border hover:bg-destructive/10'
                            : 'bg-card text-muted-foreground border-border hover:bg-secondary',
            )}>{t(`accountManager.${label}`, { count: counts[key] })}</button>
        ))}
    </div>
}
