import { useEffect, useRef, useState } from 'react'
import { Download, Upload } from 'lucide-react'
import { createAccountTransferResource } from './accountTransferResource.js'

function downloadAccounts(blob) {
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `ds2api-accounts-${new Date().toISOString().replace(/[:.]/g, '-')}.json`
    document.body.appendChild(link)
    try { link.click() } finally {
        link.remove()
        setTimeout(() => URL.revokeObjectURL(url), 1000)
    }
}

export default function AccountTransferControls({ apiFetch, t, onMessage, onRefresh, fetchAccounts, disabled = false }) {
    const [busy, setBusy] = useState('')
    const input = useRef(null)
    const resource = useRef(null)
    if (!resource.current) resource.current = createAccountTransferResource(setBusy)
    useEffect(() => {
        setBusy('')
        return () => resource.current.cancel()
    }, [apiFetch])

    const callbacks = {
        onError: detail => onMessage('error', detail || t('accountManager.transferFailed')),
        onTooLarge: () => onMessage('error', t('accountManager.importTooLarge')),
        onExported: () => onMessage('success', t('accountManager.accountsExported')),
        onImported: async (data, isCurrent) => {
            try {
                await Promise.all([fetchAccounts(1), onRefresh?.()])
                if (isCurrent()) onMessage('success', t('accountManager.accountsImported', { added: data.added, updated: data.updated }))
            } catch {
                if (isCurrent()) onMessage('error', t('accountManager.importRefreshFailed'))
            }
        },
    }
    const buttonClass = 'flex items-center gap-2 px-3 py-2 bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 transition-colors text-xs font-medium border border-border disabled:opacity-50'
    return <>
        <input ref={input} type="file" accept=".json,application/json" className="hidden" aria-label={t('accountManager.importAccounts')}
            onChange={event => {
                const file = event.target.files?.[0]
                event.target.value = ''
                void resource.current.importAccounts(file, apiFetch, callbacks)
            }} />
        <button type="button" className={buttonClass} disabled={disabled || Boolean(busy)} title={t('accountManager.importAccountsHint')}
            onClick={() => input.current?.click()}>
            <Upload className="w-4 h-4" />{t(busy === 'import' ? 'accountManager.importingAccounts' : 'accountManager.importAccounts')}
        </button>
        <button type="button" className={buttonClass} disabled={disabled || Boolean(busy)} title={t('accountManager.exportAccountsHint')}
            onClick={() => { void resource.current.exportAccounts(apiFetch, downloadAccounts, callbacks) }}>
            <Download className="w-4 h-4" />{t(busy === 'export' ? 'accountManager.exportingAccounts' : 'accountManager.exportAccounts')}
        </button>
    </>
}
