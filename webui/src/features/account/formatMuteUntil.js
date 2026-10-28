export function formatMuteUntil(muteUntil, now = new Date()) {
    if (!muteUntil || muteUntil <= 0) return '--'
    const d = new Date(muteUntil * 1000)
    if (isNaN(d.getTime())) return '--'
    const month = String(d.getMonth() + 1).padStart(2, '0')
    const day = String(d.getDate()).padStart(2, '0')
    const hour = String(d.getHours()).padStart(2, '0')
    const min = String(d.getMinutes()).padStart(2, '0')
    const isCrossYear = d.getFullYear() !== now.getFullYear()
    if (isCrossYear) {
        return `${d.getFullYear()}年${month}月${day}日 ${hour}:${min}`
    }
    return `${month}月${day}日 ${hour}:${min}`
}
