import { useRef, useState } from 'react'
import { Pencil, Play, Plus, Shield, Trash2 } from 'lucide-react'
import clsx from 'clsx'

import { useI18n } from '../../i18n'
import ProxyFormModal from './ProxyFormModal'
import useProxyOperations from './useProxyOperations'
import { EMPTY_PROXY_FORM, isManagedProxy, proxyEndpoint, proxyRecordValue } from './proxyFields'

const EMPTY_PROXIES = []

function createEmptyProxyForm() { return { ...EMPTY_PROXY_FORM } }

function ProxyStatusBadge({ t, result, testing = false }) {
    if (testing) {
        return (
            <span className="inline-flex items-center rounded-full border border-border bg-muted/40 px-2 py-1 text-[10px] font-medium text-muted-foreground">
                {t('proxyManager.testing')}
            </span>
        )
    }
    if (!result) {
        return (
            <span className="inline-flex items-center rounded-full border border-border bg-muted/20 px-2 py-1 text-[10px] font-medium text-muted-foreground">
                {t('proxyManager.untested')}
            </span>
        )
    }
    return (
        <span
            className={clsx(
                'inline-flex items-center rounded-full border px-2 py-1 text-[10px] font-medium',
                result.success
                    ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-500'
                    : 'border-destructive/20 bg-destructive/10 text-destructive'
            )}
        >
            {result.success
                ? t('proxyManager.testSuccessShort', { time: result.response_time ?? 0 })
                : t('proxyManager.testFailedShort')}
        </span>
    )
}

function ProxiesTable({
    t,
    proxies,
    testing,
    testResults,
    onCreate,
    onTest,
    onEdit,
    onDelete,
    onTestAll,
    testingAll,
}) {
    return (
        <div className="bg-card border border-border rounded-xl overflow-hidden shadow-sm">
            <div className="p-6 border-b border-border flex flex-col md:flex-row md:items-center justify-between gap-4">
                <div>
                    <h2 className="text-lg font-semibold">{t('proxyManager.title')}</h2>
                    <p className="text-sm text-muted-foreground">{t('proxyManager.desc')}</p>
                </div>
                <div className="flex gap-2">
                <button onClick={onTestAll} disabled={testingAll || proxies.length === 0} className="px-4 py-2 rounded-lg border border-border text-sm disabled:opacity-50">{t(testingAll ? 'proxyManager.testing' : 'proxyManager.testAll')}</button>
                <button
                    onClick={onCreate}
                    className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors font-medium text-sm shadow-sm"
                >
                    <Plus className="w-4 h-4" />
                    {t('proxyManager.addProxy')}
                </button>
                </div>
            </div>

            {proxies.length === 0 ? (
                <div className="p-10 text-center text-muted-foreground">{t('proxyManager.noProxies')}</div>
            ) : (
                <div className="divide-y divide-border">
                    {proxies.map((proxy) => {
                        const result = proxyRecordValue(testResults, proxy.id)
                        const managed = isManagedProxy(proxy)
                        const isTesting = Boolean(proxyRecordValue(testing, proxy.id))
                        return (
                            <div key={proxy.id} className="p-4 md:p-5 flex flex-col lg:flex-row lg:items-center justify-between gap-4 hover:bg-muted/40 transition-colors">
                                <div className="min-w-0">
                                    <div className="flex flex-wrap items-center gap-2">
                                        <div className="font-medium text-foreground">{proxy.name || proxyEndpoint(proxy)}</div>
                                        <span className="inline-flex items-center rounded-full border border-primary/20 bg-primary/10 px-2 py-1 text-[10px] font-medium uppercase tracking-wide text-primary">
                                            {proxy.type}
                                        </span>
                                        {managed && <span className="text-xs text-muted-foreground" title={t('proxyManager.managedHint')}>{t('proxyManager.managedNode')}</span>}
                                        {proxy.username && (
                                            <span className="inline-flex items-center gap-1 rounded-full border border-border bg-muted/20 px-2 py-1 text-[10px] font-medium text-muted-foreground">
                                                <Shield className="w-3 h-3" />
                                                {proxy.username}
                                            </span>
                                        )}
                                        <ProxyStatusBadge t={t} result={result} testing={isTesting} />
                                    </div>
                                    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                                        <span className="font-mono bg-muted/30 px-2 py-1 rounded border border-border">
                                            {proxyEndpoint(proxy)}
                                        </span>
                                        {proxy.has_password && (
                                            <span className="rounded-full border border-border bg-muted/20 px-2 py-1 text-[10px]">
                                                {t('proxyManager.authEnabled')}
                                            </span>
                                        )}
                                        {result?.message && (
                                            <span className="truncate max-w-full">{result.message}</span>
                                        )}
                                        {result?.tested_at && <span>{t('proxyManager.testedAt', {time:new Date(result.tested_at).toLocaleTimeString()})}</span>}
                                    </div>
                                </div>

                                <div className="flex items-center gap-2 self-start lg:self-auto">
                                    <button
                                        onClick={() => onTest(proxy)}
                                        disabled={isTesting}
                                        className="inline-flex items-center gap-1 px-3 py-1.5 rounded-md border border-border hover:bg-secondary transition-colors text-xs font-medium disabled:opacity-50"
                                    >
                                        <Play className="w-3.5 h-3.5" />
                                        {t('proxyManager.testAction')}
                                    </button>
                                    <button
                                        onClick={() => onEdit(proxy)}
                                        disabled={managed}
                                        className="p-2 text-muted-foreground hover:text-primary hover:bg-primary/10 rounded-md transition-colors"
                                        title={t(managed ? 'proxyManager.managedHint' : 'proxyManager.editProxy')}
                                    >
                                        <Pencil className="w-4 h-4" />
                                    </button>
                                    <button
                                        onClick={() => onDelete(proxy)}
                                        disabled={managed}
                                        className="p-2 text-muted-foreground hover:text-destructive hover:bg-destructive/10 rounded-md transition-colors"
                                        title={t(managed ? 'proxyManager.managedHint' : 'proxyManager.deleteProxy')}
                                    >
                                        <Trash2 className="w-4 h-4" />
                                    </button>
                                </div>
                            </div>
                        )
                    })}
                </div>
            )}
        </div>
    )
}

export default function ProxyManagerContainer({ config, onRefresh, onMessage, authFetch }) {
    const { t } = useI18n()
    const apiFetch = authFetch || fetch

    const [showModal, setShowModal] = useState(false)
    const [editingProxy, setEditingProxy] = useState(null)
    const [form, setForm] = useState(createEmptyProxyForm())
    const modalGeneration = useRef(0)

    const proxies = config?.proxies || EMPTY_PROXIES

    const openCreate = () => {
        modalGeneration.current++
        setEditingProxy(null)
        setForm(current => ({...EMPTY_PROXY_FORM, type: current.type, port: current.port}))
        setShowModal(true)
    }

    const openEdit = (proxy) => {
        if (isManagedProxy(proxy)) return
        modalGeneration.current++
        setEditingProxy(proxy)
        setForm({ ...EMPTY_PROXY_FORM, ...proxy, password: '', clear_password: false })
        setShowModal(true)
    }

    const closeModal = () => {
        modalGeneration.current++
        setShowModal(false)
        setEditingProxy(null)
        setForm(current => ({...EMPTY_PROXY_FORM, type: current.type, port: current.port}))
    }

    const {saving, testing, testResults, testingDraft, testingAll, saveProxy, deleteProxy, testProxy, testDraft, testAll, finishImport} = useProxyOperations({
        apiFetch, proxies, form, editingProxy, t, onRefresh, onMessage,
        modalGeneration: modalGeneration.current,
        getModalGeneration: () => modalGeneration.current, onSaved: closeModal,
    })

    return (
        <div className="space-y-6">
            <div className="grid gap-4 md:grid-cols-3">
                <div className="bg-card border border-border rounded-xl p-5 shadow-sm">
                    <div className="text-[10px] text-muted-foreground font-bold uppercase tracking-wider">{t('proxyManager.totalProxies')}</div>
                    <div className="mt-2 text-2xl font-bold">{proxies.length}</div>
                </div>
                <div className="bg-card border border-border rounded-xl p-5 shadow-sm">
                    <div className="text-[10px] text-muted-foreground font-bold uppercase tracking-wider">{t('proxyManager.socks5hCount')}</div>
                    <div className="mt-2 text-2xl font-bold">{proxies.filter(proxy => proxy.type === 'socks5h').length}</div>
                </div>
                <div className="bg-card border border-border rounded-xl p-5 shadow-sm">
                    <div className="text-[10px] text-muted-foreground font-bold uppercase tracking-wider">{t('proxyManager.authProxyCount')}</div>
                    <div className="mt-2 text-2xl font-bold">{proxies.filter(proxy => proxy.username || proxy.has_password).length}</div>
                </div>
            </div>

            <ProxiesTable
                t={t}
                proxies={proxies}
                testing={testing}
                testResults={testResults}
                onCreate={openCreate}
                onTest={testProxy}
                onEdit={openEdit}
                onDelete={deleteProxy}
                onTestAll={testAll}
                testingAll={testingAll}
            />

            <ProxyFormModal
                show={showModal}
                t={t}
                form={form}
                setForm={setForm}
                editingProxy={editingProxy}
                loading={saving}
                onClose={closeModal}
                onSubmit={saveProxy}
                apiFetch={apiFetch}
                onImported={finishImport}
                onTest={testDraft}
                testing={testingDraft}
            />
        </div>
    )
}
