import { Play } from 'lucide-react'
import clsx from 'clsx'

export function AccountTestButton({ t, running, hasAccounts, onStart, onCancel }) {
    return (
        <button
            onClick={running ? onCancel : onStart}
            disabled={!running && !hasAccounts}
            className="flex items-center px-3 py-2 bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 transition-colors text-xs font-medium border border-border disabled:opacity-50"
        >
            {running ? <span className="animate-spin mr-2">⟳</span> : <Play className="w-3 h-3 mr-2" />}
            {t(running ? 'actions.cancel' : 'accountManager.testAll')}
        </button>
    )
}

export function AccountTestProgress({ t, running, progress }) {
    if (!progress || (!running && !progress.stopped) || progress.total <= 0) return null
    return (
        <div className="p-4 border-b border-border bg-muted/30">
            <div className="flex items-center justify-between text-sm mb-2">
                <span className="font-medium">{t(progress.stopped ? 'accountManager.testAllStopped' : 'accountManager.testingAllAccounts', progress)}</span>
                <span className="text-muted-foreground">{progress.current} / {progress.total}</span>
            </div>
            <div className="w-full bg-muted rounded-full h-2 overflow-hidden mb-4">
                <div
                    className="bg-primary h-full transition-all duration-300"
                    style={{ width: `${(progress.current / progress.total) * 100}%` }}
                />
            </div>
            {progress.results.length > 0 && (
                <div className="grid grid-cols-2 md:grid-cols-4 gap-2 max-h-32 overflow-y-auto custom-scrollbar">
                    {progress.results.map((result, index) => (
                        <div key={index} className={clsx(
                            "text-xs px-2 py-1 rounded border truncate",
                            result.success ? "bg-emerald-500/10 border-emerald-500/20 text-emerald-500" : "bg-destructive/10 border-destructive/20 text-destructive"
                        )}>
                            {result.success ? '✓' : '✗'} {result.id}
                        </div>
                    ))}
                </div>
            )}
        </div>
    )
}
