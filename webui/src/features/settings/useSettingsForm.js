import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import {
    fetchSettings,
    getExportData,
    postImportData,
    postPassword,
    putSettings,
} from './settingsApi.js'

import { fromServerForm, parseJSONMap, toServerPayload } from './settingsMapping.js'
import { createLatestRequest, createRequestGroup } from '../../utils/latestRequest.js'

const MAX_AUTO_FETCH_FAILURES = 3

const DEFAULT_FORM = {
    model_aliases_text: '{}',
    // Registered settings (internal/config/settings.go), keyed by their registry
    // key and then by field name, and registered sections (sections.go), keyed
    // by their config key. Both are built from the server's setting_specs /
    // section_specs payload, so a new setting or section needs no change here.
    settings: {},
}

export function useSettingsForm({ apiFetch, t, onMessage, onRefresh, onForceLogout, isVercel = false }) {
    const currentUI = useRef({ t, onMessage, onRefresh })
    currentUI.current = { t, onMessage, onRefresh }
    const paused = useRef(false)
    const reads = useMemo(createLatestRequest, [apiFetch])
    const writes = useMemo(createRequestGroup, [apiFetch])
    const [loading, setLoading] = useState(false)
    const [saving, setSaving] = useState(false)
    const [changingPassword, setChangingPassword] = useState(false)
    const [importing, setImporting] = useState(false)
    const [exportData, setExportData] = useState(null)
    const [importMode, setImportMode] = useState('merge')
    const [importText, setImportText] = useState('')
    const [newPassword, setNewPassword] = useState('')
    const [consecutiveFailures, setConsecutiveFailures] = useState(0)
    const [autoFetchPaused, setAutoFetchPaused] = useState(false)
    const [lastError, setLastError] = useState('')
    const [settingsMeta, setSettingsMeta] = useState({
        default_password_warning: false,
        env_backed: false,
        needs_vercel_sync: false,
    })
    const [form, setForm] = useState(DEFAULT_FORM)
    // The registries as reported by the settings API: what to render and how to
    // label it, straight from internal/config/settings.go and sections.go.
    const [settingSpecs, setSettingSpecs] = useState([])
    const [sectionSpecs, setSectionSpecs] = useState([])
    // Which settings share a card, and where each card sits on the page.
    const [settingCardSpecs, setSettingCardSpecs] = useState([])

    const trackLoadFailure = useCallback(() => {
        setConsecutiveFailures((prev) => {
            const next = prev + 1
            if (isVercel && next >= MAX_AUTO_FETCH_FAILURES) {
                paused.current = true
                setAutoFetchPaused(true)
            }
            return next
        })
    }, [isVercel])

    const loadSettings = useCallback(async ({ manual = false } = {}) => {
        if (isVercel && paused.current && !manual) {
            return
        }
        setLoading(true)
        const request = reads.start()
        const { t, onMessage } = currentUI.current
        try {
            const { res, data } = await fetchSettings(apiFetch, t, request.signal)
            if (!request.isCurrent()) return
            if (!res.ok) {
                const detail = data.detail || t('settings.loadFailed')
                setLastError(detail)
                onMessage('error', detail)
                trackLoadFailure()
                return
            }
            setConsecutiveFailures(0)
            paused.current = false
            setAutoFetchPaused(false)
            setLastError('')
            setSettingsMeta({
                default_password_warning: Boolean(data.admin?.default_password_warning),
                env_backed: Boolean(data.env_backed),
                needs_vercel_sync: Boolean(data.needs_vercel_sync),
            })
            setForm(fromServerForm(data, data.section_specs))
            setSettingSpecs(Array.isArray(data.setting_specs) ? data.setting_specs : [])
            setSectionSpecs(Array.isArray(data.section_specs) ? data.section_specs : [])
            setSettingCardSpecs(Array.isArray(data.setting_cards) ? data.setting_cards : [])
        } catch (e) {
            if (!request.isCurrent()) return
            const detail = e?.message || t('settings.loadFailed')
            setLastError(detail)
            onMessage('error', detail)
            trackLoadFailure()
            // eslint-disable-next-line no-console
            console.error(e)
        } finally {
            if (request.isCurrent()) setLoading(false)
        }
    }, [apiFetch, isVercel, reads, trackLoadFailure])

    useEffect(() => {
        paused.current = false
        setSaving(false)
        loadSettings()
        return () => { reads.cancel(); writes.cancel() }
    }, [loadSettings, reads, writes])

    const retryLoadSettings = useCallback(() => {
        paused.current = false
        setAutoFetchPaused(false)
        loadSettings({ manual: true })
    }, [loadSettings])

    const saveSettings = useCallback(async () => {
        if (writes.has('settings')) return
        let modelAliases = {}
        try {
            modelAliases = parseJSONMap(form.model_aliases_text, 'model_aliases', t)
        } catch (e) {
            onMessage('error', e.message)
            return
        }

        const payload = {
            ...toServerPayload(form, sectionSpecs, settingSpecs),
            model_aliases: modelAliases,
        }

        const request = writes.start('settings')
        reads.cancel()
        setSaving(true)
        try {
            const { res, data } = await putSettings(apiFetch, payload, request.signal)
            if (!request.isCurrent()) return
            if (!res.ok) {
                onMessage('error', data.detail || t('settings.saveFailed'))
                return
            }
            currentUI.current.onMessage('success', currentUI.current.t('settings.saveSuccess'))
            currentUI.current.onRefresh?.()
            await loadSettings()
        } catch (e) {
            if (!request.isCurrent()) return
            onMessage('error', t('settings.saveFailed'))
            // eslint-disable-next-line no-console
            console.error(e)
        } finally {
            if (request.isCurrent()) setSaving(false)
            request.finish()
        }
    }, [apiFetch, form, loadSettings, onMessage, reads, sectionSpecs, settingSpecs, t, writes])

    const updatePassword = useCallback(async () => {
        if (String(newPassword || '').trim().length < 4) {
            onMessage('error', t('settings.passwordTooShort'))
            return
        }
        setChangingPassword(true)
        try {
            const { res, data } = await postPassword(apiFetch, newPassword.trim())
            if (!res.ok) {
                onMessage('error', data.detail || t('settings.passwordUpdateFailed'))
                return
            }
            onMessage('success', t('settings.passwordUpdated'))
            setNewPassword('')
            if (typeof onForceLogout === 'function') {
                onForceLogout()
            }
        } catch (_e) {
            onMessage('error', t('settings.passwordUpdateFailed'))
        } finally {
            setChangingPassword(false)
        }
    }, [apiFetch, newPassword, onForceLogout, onMessage, t])

    const loadExportData = useCallback(async () => {
        try {
            const { res, data } = await getExportData(apiFetch)
            if (!res.ok) {
                onMessage('error', data.detail || t('settings.exportFailed'))
                return null
            }
            setExportData(data)
            onMessage('success', t('settings.exportLoaded'))
            return data
        } catch (_e) {
            onMessage('error', t('settings.exportFailed'))
            return null
        }
    }, [apiFetch, onMessage, t])

    const downloadExportFile = useCallback(async () => {
        let latest = exportData
        if (!latest?.json) {
            const loaded = await loadExportData()
            if (!loaded) {
                return
            }
            latest = loaded
        }
        const jsonText = String(latest?.json || '').trim()
        if (!jsonText) {
            onMessage('error', t('settings.exportFailed'))
            return
        }
        const blob = new Blob([jsonText], { type: 'application/json;charset=utf-8' })
        const url = URL.createObjectURL(blob)
        const now = new Date()
        const pad = (n) => String(n).padStart(2, '0')
        const filename = `ds2api-config-backup-${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}.json`
        const link = document.createElement('a')
        link.href = url
        link.download = filename
        document.body.appendChild(link)
        link.click()
        document.body.removeChild(link)
        URL.revokeObjectURL(url)
        onMessage('success', t('settings.exportDownloaded'))
    }, [exportData, loadExportData, onMessage, t])

    const loadImportFile = useCallback((file) => {
        if (!file) return
        const reader = new FileReader()
        reader.onload = () => {
            const text = String(reader.result || '')
            setImportText(text)
            onMessage('success', t('settings.importFileLoaded'))
        }
        reader.onerror = () => {
            onMessage('error', t('settings.importFileReadFailed'))
        }
        reader.readAsText(file, 'utf-8')
    }, [onMessage, t])

    const doImport = useCallback(async () => {
        if (!String(importText || '').trim()) {
            onMessage('error', t('settings.importEmpty'))
            return
        }
        let parsed
        try {
            parsed = JSON.parse(importText)
        } catch (_e) {
            onMessage('error', t('settings.importInvalidJson'))
            return
        }
        setImporting(true)
        try {
            const { res, data } = await postImportData(apiFetch, importMode, parsed)
            if (!res.ok) {
                onMessage('error', data.detail || t('settings.importFailed'))
                return
            }
            onMessage('success', t('settings.importSuccess', { mode: importMode }))
            if (typeof onRefresh === 'function') {
                onRefresh()
            }
            await loadSettings()
        } catch (_e) {
            onMessage('error', t('settings.importFailed'))
        } finally {
            setImporting(false)
        }
    }, [apiFetch, importMode, importText, loadSettings, onMessage, onRefresh, t])

    const syncHintVisible = useMemo(
        () => settingsMeta.env_backed || settingsMeta.needs_vercel_sync,
        [settingsMeta.env_backed, settingsMeta.needs_vercel_sync],
    )

    return {
        form,
        setForm,
        settingSpecs,
        sectionSpecs,
        settingCardSpecs,
        loading,
        saving,
        changingPassword,
        importing,
        exportData,
        importMode,
        setImportMode,
        importText,
        setImportText,
        newPassword,
        setNewPassword,
        consecutiveFailures,
        autoFetchPaused,
        lastError,
        settingsMeta,
        syncHintVisible,
        retryLoadSettings,
        saveSettings,
        updatePassword,
        loadExportData,
        downloadExportFile,
        loadImportFile,
        doImport,
    }
}
