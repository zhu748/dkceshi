import { formatMuteUntil } from './formatMuteUntil.js'

export function isAccountCoolingDown(account, now = Date.now()) {
    const until = Number(account?.cooldown_until)
    if (Number.isFinite(until) && until > 0) return until * 1000 > now
    return account?.cooling_down === true
}

export function formatCooldownUntil(until) {
    const date = new Date(Number(until) * 1000)
    if (!Number.isFinite(date.getTime()) || Number(until) <= 0) return '--'
    return `${formatMuteUntil(Number(until))}:${String(date.getSeconds()).padStart(2, '0')}`
}
