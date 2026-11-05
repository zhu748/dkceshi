export const PROXY_TYPES = ['http', 'https', 'socks4', 'socks4a', 'socks5', 'socks5h', 'turn', 'turns', 'sstp']
export const DEFAULT_PORTS = {http: 80, https: 443, socks4: 1080, socks4a: 1080, socks5: 1080, socks5h: 1080, turn: 3478, turns: 5349, sstp: 443}
export const EMPTY_PROXY_FORM = {name: '', type: 'socks5h', host: '', port: 1080, username: '', password: '', tls_server_name: '', ca_file: '', insecure: false, realm: '', mtu: '', clear_password: false}

export function proxyEndpoint(proxy) { const host = String(proxy.host || ''); return `${host.includes(':') ? `[${host}]` : host}:${proxy.port}` }

export function isManagedProxy(proxy) { return String(proxy?.id || '').startsWith('mihomo-') }
export function proxyRecordValue(record, id) { return Object.hasOwn(record, id) ? record[id] : undefined }
