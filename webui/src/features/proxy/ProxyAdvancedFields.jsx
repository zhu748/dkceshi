export default function ProxyAdvancedFields({t, form, setForm}) {
    const tls = ['https', 'turns', 'sstp'].includes(form.type)
    const turn = ['turn', 'turns'].includes(form.type)
    const update = (key, value) => setForm(current => ({...current, [key]: value}))
    return <>
        {['socks4', 'socks4a'].includes(form.type) && <p className="text-xs text-amber-500">{t('proxyManager.socks4Hint')}</p>}
        {(turn || form.type === 'sstp') && <p className="text-xs text-muted-foreground">{t(`proxyManager.${turn ? 'turnHint' : 'sstpHint'}`)}</p>}
        {(tls || turn || form.type === 'sstp') && <details className="rounded-lg border border-border p-3">
            <summary className="cursor-pointer text-sm font-medium">{t('proxyManager.advanced')}</summary>
            <div className="grid gap-3 md:grid-cols-2 mt-3">
                {tls && <>
                    <label className="text-sm">SNI<input className="input-field mt-1" value={form.tls_server_name || ''} onChange={e => update('tls_server_name', e.target.value)} /></label>
                    <label className="text-sm">{t('proxyManager.caFile')}<input className="input-field mt-1" value={form.ca_file || ''} onChange={e => update('ca_file', e.target.value)} /></label>
                    <label className="flex items-center gap-2 text-xs md:col-span-2"><input type="checkbox" checked={Boolean(form.insecure)} onChange={e => update('insecure', e.target.checked)} />{t('proxyManager.insecure')}</label>
                </>}
                {turn && <label className="text-sm">Realm<input className="input-field mt-1" value={form.realm || ''} onChange={e => update('realm', e.target.value)} /></label>}
                {form.type === 'sstp' && <label className="text-sm">MTU<input type="number" min="576" max="9000" placeholder="1400" className="input-field mt-1" value={form.mtu || ''} onChange={e => update('mtu', e.target.value)} /></label>}
            </div>
        </details>}
    </>
}
