export async function readProxyResponse(res, t) {
    const raw = await res.text()
    const text = raw.trim()
    if (!text && res.ok) return {}
    if (String(res.headers.get('content-type') || '').toLowerCase().includes('application/json')) {
        try { return JSON.parse(text) } catch { /* Use the controlled fallback below. */ }
    }
    if (!res.ok) return {detail: text || t('messages.requestFailed')}
    throw new Error(t('settings.nonJsonResponse', {status: res.status}))
}

export function proxyPayload(form) {
    const port = Number(form.port)
    if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('proxyManager.invalidPort')
    const mtu = form.mtu === '' || form.mtu == null || form.mtu === 0 ? 0 : Number(form.mtu)
    if (!Number.isInteger(mtu) || (mtu !== 0 && (mtu < 576 || mtu > 9000)) || form.mtu === '0') throw new Error('proxyManager.invalidMTU')
    return {
        name: form.name, type: form.type, host: form.host, port,
        username: form.username, password: form.password,
        clear_password: Boolean(form.clear_password),
        tls_server_name: form.tls_server_name, ca_file: form.ca_file,
        insecure: Boolean(form.insecure), realm: form.realm, mtu,
    }
}
