import { useState } from 'react'
import { X } from 'lucide-react'
import ProxyPasteImport from './ProxyPasteImport'
import ProxyAdvancedFields from './ProxyAdvancedFields'
import { PROXY_TYPES, DEFAULT_PORTS } from './proxyFields'

export default function ProxyFormModal({
    show,
    t,
    form,
    setForm,
    editingProxy,
    loading,
    onClose,
    onSubmit,
    apiFetch,
    onImported,
}) {
    const [pasteBlocksSave, setPasteBlocksSave] = useState(false)
    if (!show) {
        return null
    }

    const isEditing = Boolean(editingProxy?.id)

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4 animate-in fade-in">
            <div className="bg-card w-full max-w-2xl max-h-[90vh] overflow-y-auto rounded-xl border border-border shadow-2xl animate-in zoom-in-95">
                <div className="p-4 border-b border-border flex justify-between items-center">
                    <div>
                        <h3 className="font-semibold">
                            {isEditing ? t('proxyManager.modalEditTitle') : t('proxyManager.modalAddTitle')}
                        </h3>
                        <p className="text-xs text-muted-foreground mt-1">
                            {t('proxyManager.modalDesc')}
                        </p>
                    </div>
                    <button onClick={onClose} className="text-muted-foreground hover:text-foreground">
                        <X className="w-5 h-5" />
                    </button>
                </div>

                <div className="p-6 space-y-4">
 <ProxyPasteImport t={t} apiFetch={apiFetch} form={form} setForm={setForm} isEditing={isEditing} onImported={onImported} onBlocksSave={setPasteBlocksSave} />
                    <div className="grid md:grid-cols-2 gap-4">
                        <div>
                            <label className="block text-sm font-medium mb-1.5">{t('proxyManager.nameLabel')}</label>
                            <input
                                type="text"
                                className="input-field"
                                placeholder={t('proxyManager.namePlaceholder')}
                                value={form.name}
                                onChange={e => setForm({ ...form, name: e.target.value })}
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium mb-1.5">{t('proxyManager.typeLabel')}</label>
                            <select
                                className="input-field"
                                value={form.type}
                                onChange={e => {const type = e.target.value; setForm({ ...form, type, port: DEFAULT_PORTS[type], tls_server_name: '', ca_file: '', insecure: false, realm: '', mtu: '' })}}
                            >
                                {PROXY_TYPES.map(type => <option key={type} value={type}>{type.toUpperCase()}</option>)}
                            </select>
                        </div>
                    </div>

                    <div className="grid md:grid-cols-[1fr_128px] gap-4">
                        <div>
                            <label className="block text-sm font-medium mb-1.5">{t('proxyManager.hostLabel')}</label>
                            <input
                                type="text"
                                className="input-field"
                                placeholder={t('proxyManager.hostPlaceholder')}
                                value={form.host}
                                onChange={e => setForm({ ...form, host: e.target.value })}
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium mb-1.5">{t('proxyManager.portLabel')}</label>
                            <input
                                type="number"
                                min="1"
                                max="65535"
                                className="input-field"
                                value={form.port}
                                onChange={e => setForm({ ...form, port: Number(e.target.value) || '' })}
                            />
                        </div>
                    </div>

                    <div className="grid md:grid-cols-2 gap-4">
                        <div>
                            <label className="block text-sm font-medium mb-1.5">{t('proxyManager.usernameLabel')}</label>
                            <input
                                type="text"
                                className="input-field"
                                placeholder={t('proxyManager.usernamePlaceholder')}
                                value={form.username}
                                onChange={e => setForm({ ...form, username: e.target.value })}
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium mb-1.5">{t('proxyManager.passwordLabel')}</label>
                            <input
                                type="password"
                                className="input-field bg-[#09090b]"
                                placeholder={t('proxyManager.passwordPlaceholder')}
                                value={form.password}
                                onChange={e => setForm({ ...form, password: e.target.value })}
                            />
                            {isEditing && (
                                <p className="mt-1 text-[11px] text-muted-foreground">{t('proxyManager.passwordKeepHint')}</p>
                            )}
                        </div>
                    </div>

                    {isEditing && <label className="flex gap-2 text-sm"><input type="checkbox" checked={Boolean(form.clear_password)} onChange={e => setForm({...form, clear_password: e.target.checked, password: ''})} />{t('proxyManager.clearPassword')}</label>}
 <ProxyAdvancedFields t={t} form={form} setForm={setForm} />
 <div className="rounded-lg border border-border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
                        {t('proxyManager.typeHelp')}
                    </div>

                    <div className="flex justify-end gap-2 pt-2">
                        <button
                            onClick={onClose}
                            className="px-4 py-2 rounded-lg border border-border hover:bg-secondary transition-colors text-sm font-medium"
                        >
                            {t('actions.cancel')}
                        </button>
                        <button
                            onClick={onSubmit}
                            disabled={loading || pasteBlocksSave}
                            className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-sm font-medium disabled:opacity-50"
                        >
                            {loading
                                ? t('proxyManager.saving')
                                : (isEditing ? t('proxyManager.saveEdit') : t('proxyManager.saveAdd'))}
                        </button>
                    </div>
                </div>
            </div>
        </div>
    )
}
