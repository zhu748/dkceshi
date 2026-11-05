import {useEffect, useRef, useState} from 'react'
import {proxyPayload, readProxyResponse} from './proxyApi'
import {isManagedProxy, proxyEndpoint} from './proxyFields'

export default function useProxyOperations(props) {
    const {apiFetch, proxies} = props
    const latest = useRef(props)
    latest.current = props
    const alive = useRef(false)
    const operations = useRef(new Set())
    const savingRequest = useRef(null)
    const testRequests = useRef(new Map())
    const deleteRequests = useRef(new Map())
    const [saving, setSaving] = useState(false)
    const [testing, setTesting] = useState({})
    const [testResults, setTestResults] = useState({})
    useEffect(() => {
        alive.current = true
        setSaving(false)
        return () => {
            alive.current = false
            for (const operation of operations.current) operation.controller.abort()
            operations.current.clear()
            savingRequest.current = null
            testRequests.current.clear()
            deleteRequests.current.clear()
        }
    }, [apiFetch])
    useEffect(() => {
        for (const operation of testRequests.current.values()) operation.controller.abort()
        testRequests.current.clear()
        setTesting({})
        setTestResults({})
    }, [proxies, apiFetch])
    const start = () => {
        const operation = {controller: new AbortController(), source: apiFetch}
        operations.current.add(operation)
        return operation
    }
    const active = operation => alive.current && !operation.controller.signal.aborted && operation.source === latest.current.apiFetch
    const finish = operation => operations.current.delete(operation)
    const message = (kind, value) => latest.current.onMessage(kind, value)
    const invalidateTest = id => {
        testRequests.current.get(id)?.controller.abort()
        testRequests.current.delete(id)
        setTesting(previous => {const next = {...previous}; delete next[id]; return next})
        setTestResults(previous => {const next = {...previous}; delete next[id]; return next})
    }
    const refresh = async (operation, snapshot) => {
        try {await snapshot.onRefresh?.(); return true}
        catch {if (active(operation)) message('error', snapshot.t('proxyManager.savedRefreshFailed')); return false}
    }
    const saveProxy = async () => {
        if (savingRequest.current) return
        const snapshot = latest.current
        const {form, editingProxy, t} = snapshot
        if (!form.host?.trim()) {message('error', t('proxyManager.requiredFields')); return}
        let payload
        try {payload = proxyPayload(form)} catch (err) {message('error', t(err.message)); return}
        const operation = start()
        savingRequest.current = operation
        const generation = snapshot.getModalGeneration()
        setSaving(true)
        try {
            const res = await apiFetch(editingProxy?.id ? `/admin/proxies/${encodeURIComponent(editingProxy.id)}` : '/admin/proxies', {
                method: editingProxy?.id ? 'PUT' : 'POST', signal: operation.controller.signal,
                headers: {'Content-Type': 'application/json'}, body: JSON.stringify(payload),
            })
            const data = await readProxyResponse(res, t)
            if (!active(operation)) return
            if (!res.ok) {message('error', data.detail || t('messages.requestFailed')); return}
            invalidateTest(editingProxy?.id || data.proxy?.id)
            const refreshed = await refresh(operation, snapshot)
            if (!active(operation)) return
            if (refreshed) message('success', t(editingProxy?.id ? 'proxyManager.updateSuccess' : data.skipped ? 'proxyManager.duplicateSkipped' : 'proxyManager.addSuccess'))
            if (latest.current.getModalGeneration() === generation) latest.current.onSaved()
        } catch (err) {if (active(operation)) message('error', err.message || t('messages.networkError'))}
        finally {
            finish(operation)
            if (savingRequest.current === operation) {savingRequest.current = null; if (active(operation)) setSaving(false)}
        }
    }
    const deleteProxy = async proxy => {
        if (isManagedProxy(proxy) || deleteRequests.current.has(proxy.id)) return
        const snapshot = latest.current
        const {t} = snapshot
        if (!confirm(t('proxyManager.deleteConfirm', {name: proxy.name || proxyEndpoint(proxy)}))) return
        const operation = start()
        deleteRequests.current.set(proxy.id, operation)
        try {
            const res = await apiFetch(`/admin/proxies/${encodeURIComponent(proxy.id)}`, {method:'DELETE', signal:operation.controller.signal})
            const data = await readProxyResponse(res, t)
            if (!active(operation)) return
            if (!res.ok) {message('error', data.detail || t('messages.deleteFailed')); return}
            invalidateTest(proxy.id)
            if (await refresh(operation, snapshot) && active(operation)) message('success', t('messages.deleted'))
        } catch (err) {if (active(operation)) message('error', err.message || t('messages.networkError'))}
        finally {finish(operation); if (deleteRequests.current.get(proxy.id) === operation) deleteRequests.current.delete(proxy.id)}
    }
    const testProxy = async proxy => {
        if (testRequests.current.has(proxy.id)) return
        const {t} = latest.current
        const operation = start()
        testRequests.current.set(proxy.id, operation)
        setTesting(previous => ({...previous, [proxy.id]:true}))
        try {
            const res = await apiFetch('/admin/proxies/test', {method:'POST', signal:operation.controller.signal, headers:{'Content-Type':'application/json'}, body:JSON.stringify({proxy_id:proxy.id})})
            const data = await readProxyResponse(res, t)
            if (!active(operation) || testRequests.current.get(proxy.id) !== operation) return
            if (!res.ok) {message('error', data.detail || t('messages.requestFailed')); return}
            setTestResults(previous => ({...previous, [proxy.id]:data}))
            message(data.success ? 'success' : 'error', data.message || t('messages.requestFailed'))
        } catch (err) {if (active(operation)) message('error', err.message || t('messages.networkError'))}
        finally {
            finish(operation)
            if (testRequests.current.get(proxy.id) === operation) {testRequests.current.delete(proxy.id); if (active(operation)) setTesting(previous => ({...previous,[proxy.id]:false}))}
        }
    }
    const finishImport = async data => {
        const snapshot = latest.current
        const generation = snapshot.getModalGeneration()
        const operation = start()
        try {
            const refreshed = await refresh(operation, snapshot)
            if (!active(operation)) return
            if (refreshed) message('success', snapshot.t('proxyManager.importSuccess', {added:data.added, skipped:data.skipped}))
            if (latest.current.getModalGeneration() === generation) latest.current.onSaved()
        } finally {finish(operation)}
    }
    return {saving, testing, testResults, saveProxy, deleteProxy, testProxy, finishImport}
}
