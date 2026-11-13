export async function parseJSONResponse(res, t) {
    const contentType = String(res.headers.get('content-type') || '').toLowerCase()
    if (!contentType.includes('application/json')) {
        throw new Error(t('settings.nonJsonResponse', { status: res.status }))
    }
    return res.json()
}

export async function fetchSettings(apiFetch, t, signal) {
    const res = await apiFetch('/admin/settings', { signal })
    const data = await parseJSONResponse(res, t)
    return { res, data }
}

export async function putSettings(apiFetch, payload, signal) {
    const res = await apiFetch('/admin/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    })
    const data = await res.json()
    return { res, data }
}

export async function postPassword(apiFetch, newPassword, signal) {
    const res = await apiFetch('/admin/settings/password', {
        method: 'POST',
        signal,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ new_password: newPassword }),
    })
    const data = await res.json()
    return { res, data }
}

export async function getExportData(apiFetch, signal) {
    const res = await apiFetch('/admin/config/export', { signal })
    const data = await res.json()
    return { res, data }
}

export async function postImportData(apiFetch, mode, config, signal) {
    const res = await apiFetch(`/admin/config/import?mode=${encodeURIComponent(mode)}`, {
        method: 'POST',
        signal,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ config, mode }),
    })
    const data = await res.json()
    return { res, data }
}
