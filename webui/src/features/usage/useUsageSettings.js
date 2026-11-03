import { useCallback, useEffect, useRef, useState } from 'react'
import { useI18n } from '../../i18n'
import { createLatestRequest } from '../../utils/latestRequest.js'
import { applyServerSettings, loadLocalPricing, peakWindowError, persistLocalPricing, toServerUsageSettings } from './usagePricing'

export function useUsageSettings({ authFetch, onMessage }) {
    const { t } = useI18n()
    const [settingsOpen, setSettingsOpenState] = useState(false)
    const [pricingOpen, setPricingOpen] = useState(false)
    const [settingsLoadState, setSettingsLoadState] = useState('loading')
    const [usageEnabled, setUsageEnabled] = useState(false)
    const [pricing, setPricing] = useState(() => loadLocalPricing())
    const [saving, setSaving] = useState(false)
    const loadRequests = useRef(createLatestRequest()).current
    const saveRequests = useRef(createLatestRequest()).current
    const activeSave = useRef(null)
    const dialogGeneration = useRef(0)
    const mountedRef = useRef(true)
    const tRef = useRef(t)
    const onMessageRef = useRef(onMessage)

    useEffect(() => {
        tRef.current = t
        onMessageRef.current = onMessage
    })

    useEffect(() => {
        mountedRef.current = true
        setSaving(false)
        return () => {
            mountedRef.current = false
            loadRequests.cancel()
            saveRequests.cancel()
            activeSave.current = null
            ++dialogGeneration.current
        }
    }, [authFetch, loadRequests, saveRequests])

    useEffect(() => {
        const timer = setTimeout(() => persistLocalPricing(pricing), 300)
        return () => clearTimeout(timer)
    }, [pricing])

    const cancelSave = useCallback(() => {
        ++dialogGeneration.current
        saveRequests.cancel()
        activeSave.current = null
        if (mountedRef.current) setSaving(false)
    }, [saveRequests])

    // A new dialog owns its saves even when an old fetch ignores abort.
    const setSettingsOpen = useCallback((value) => {
        cancelSave()
        setSettingsOpenState(value)
    }, [cancelSave])

    const loadUsageSettings = useCallback(async () => {
        const request = loadRequests.start()
        setSettingsLoadState('loading')
        try {
            const res = await authFetch('/admin/usage/settings', { signal: request.signal })
            if (!request.isCurrent() || !mountedRef.current) return
            const data = await res.json().catch(() => ({}))
            if (!request.isCurrent() || !mountedRef.current) return
            if (!res.ok || !data.settings) throw new Error(tRef.current('usage.settingsLoadFailed'))
            setUsageEnabled(!!data.settings.enabled)
            setSettingsLoadState('ready')
            setPricing((prev) => applyServerSettings(prev, data.settings))
        } catch (error) {
            if (request.isCurrent() && mountedRef.current) {
                setSettingsLoadState('error')
                onMessageRef.current?.('error', tRef.current('usage.settingsLoadFailed'))
            }
        }
    }, [authFetch, loadRequests])

    useEffect(() => { loadUsageSettings() }, [loadUsageSettings])

    const saveUsageSettings = useCallback((nextPricing, nextEnabled) => {
        if (!mountedRef.current) return Promise.resolve(false)
        if (activeSave.current) return activeSave.current.promise
        // A pre-save read must not overwrite the accepted save later.
        loadRequests.cancel()
        const request = saveRequests.start()
        const operation = { request, promise: null }
        activeSave.current = operation
        setSaving(true)
        // Publish the operation before invoking user-supplied fetch, including
        // fetchers which throw synchronously or callbacks which start a save.
        operation.promise = Promise.resolve().then(async () => {
            const current = () => request.isCurrent() && mountedRef.current
            try {
                if (!current()) return false
                const res = await authFetch('/admin/usage/settings', {
                    method: 'PUT', signal: request.signal,
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ settings: toServerUsageSettings(nextPricing, nextEnabled) }),
                })
                if (!current()) return false
                const data = await res.json().catch(() => ({}))
                if (!current()) return false
                if (!res.ok) throw new Error(data?.detail || tRef.current('usage.saveFailed'))
                if (data.settings) {
                    setUsageEnabled(!!data.settings.enabled)
                    setPricing((prev) => applyServerSettings(prev, data.settings))
                    setSettingsLoadState('ready')
                }
                onMessageRef.current?.('success', tRef.current('usage.settingsSaved'))
                return current()
            } catch (error) {
                if (current()) onMessageRef.current?.('error', error.message || tRef.current('usage.saveFailed'))
                return false
            } finally {
                if (activeSave.current === operation) {
                    activeSave.current = null
                    if (current()) setSaving(false)
                }
            }
        })
        return operation.promise
    }, [authFetch, loadRequests, saveRequests])

    const dismissSettings = useCallback(() => {
        cancelSave()
        setPricingOpen(false)
        setSettingsOpenState(false)
    }, [cancelSave])

    const closeSettings = useCallback(async (attemptSave) => {
        const generation = dialogGeneration.current
        if (attemptSave) {
            if (settingsLoadState === 'loading') {
                onMessageRef.current?.('error', tRef.current('usage.settingsNotLoaded'))
                return
            }
            if (settingsLoadState === 'error') {
                dismissSettings()
                onMessageRef.current?.('error', tRef.current('usage.settingsNotSaved'))
                return
            }
            const peakError = peakWindowError(pricing)
            if (peakError) {
                setPricingOpen(true)
                onMessageRef.current?.('error', tRef.current(peakError))
                return
            }
            const ok = await saveUsageSettings(pricing, usageEnabled)
            if (!ok || generation !== dialogGeneration.current || !mountedRef.current) return
        }
        dismissSettings()
    }, [settingsLoadState, pricing, usageEnabled, saveUsageSettings, dismissSettings])

    return {
        settingsOpen, setSettingsOpen, pricingOpen, setPricingOpen, settingsLoadState,
        saving, usageEnabled, setUsageEnabled, pricing, setPricing,
        loadUsageSettings, saveUsageSettings, closeSettings, dismissSettings,
    }
}
