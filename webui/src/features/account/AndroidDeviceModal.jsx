import { useState } from 'react'
import { X } from 'lucide-react'

const fields = ['device_model', 'device_brand', 'android_api_level', 'device_id', 'installation_id', 'android_id', 'rangers_id', 'locale', 'timezone', 'photo_compress_ratio', 'picture_compress_format']

export default function AndroidDeviceModal({ accounts, apiFetch, t, onClose, onSaved, onMessage }) {
    const [selected, setSelected] = useState(null)
    const [draft, setDraft] = useState({})
    const [saving, setSaving] = useState(false)
    const edit = (account) => {
        setSelected(account.identifier)
        setDraft(Object.fromEntries(fields.map(key => [key, account[key] ?? ''])))
    }
    const save = async (event) => {
        event.preventDefault()
        setSaving(true)
        try {
            const response = await apiFetch(`/admin/accounts/${encodeURIComponent(selected)}`, {
                method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ...draft, photo_compress_ratio: draft.photo_compress_ratio === '' ? null : Number(draft.photo_compress_ratio) }),
            })
            const data = await response.json()
            if (!response.ok) throw new Error(data.detail || t('messages.requestFailed'))
            await onSaved()
            setSelected(null)
            onMessage('success', t('accountManager.androidDeviceSaved'))
        } catch (error) {
            onMessage('error', error.message || t('messages.networkError'))
        } finally {
            setSaving(false)
        }
    }
    return (
        <div className="fixed inset-0 z-50 bg-black/50 p-4 flex items-center justify-center">
            <div role="dialog" aria-modal="true" aria-labelledby="android-device-title" className="bg-card border border-border rounded-xl w-full max-w-3xl max-h-[85vh] overflow-y-auto p-6 space-y-4">
                <div className="flex justify-between items-center gap-3">
                    <h3 id="android-device-title" className="font-semibold">{t('accountManager.androidDeviceTitle')}</h3>
                    <button onClick={onClose} disabled={saving} aria-label={t('accountManager.realDeviceIdClose')} className="p-2 rounded-lg hover:bg-muted"><X size={20} /></button>
                </div>
                <p className="text-sm text-muted-foreground">{t('accountManager.androidDeviceDescription')}</p>
                <div className="space-y-2">
                    {accounts.map(account => (
                        <button key={account.identifier} onClick={() => edit(account)} disabled={saving} className="block w-full text-left rounded-lg border border-border p-3 hover:bg-muted">
                            <div className="font-medium break-all">{account.identifier}</div>
                            <div className="text-sm text-muted-foreground">{account.device_model || t('accountManager.androidDevicePending')} · API {account.android_api_level || '—'}</div>
                        </button>
                    ))}
                </div>
                {selected && (
                    <form onSubmit={save} className="border-t border-border pt-4 space-y-3">
                        <p className="font-medium break-all">{selected}</p>
                        {fields.map(key => (
                            <label key={key} className="block text-sm space-y-1">
                                <span>{t(`accountManager.androidDeviceFields.${key}`)}</span>
                                <input type={key === 'photo_compress_ratio' ? 'number' : 'text'} min={key === 'photo_compress_ratio' ? 0 : undefined} max={key === 'photo_compress_ratio' ? 1 : undefined} step={key === 'photo_compress_ratio' ? 0.01 : undefined} value={draft[key] ?? ''} onChange={e => setDraft(previous => ({ ...previous, [key]: e.target.value }))} disabled={saving} className="w-full bg-background border border-border rounded-lg px-3 py-2" />
                            </label>
                        ))}
                        <button type="submit" disabled={saving} className="px-4 py-2 bg-primary text-primary-foreground rounded-lg disabled:opacity-50">{saving ? '…' : t('accountManager.androidDeviceSave')}</button>
                    </form>
                )}
            </div>
        </div>
    )
}
