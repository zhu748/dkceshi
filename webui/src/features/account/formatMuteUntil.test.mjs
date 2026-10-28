import assert from 'node:assert/strict'
import test from 'node:test'

import { formatMuteUntil } from './formatMuteUntil.js'

test('formatMuteUntil handles empty and invalid inputs', () => {
    assert.equal(formatMuteUntil(0), '--')
    assert.equal(formatMuteUntil(-1), '--')
    assert.equal(formatMuteUntil(null), '--')
    assert.equal(formatMuteUntil(undefined), '--')
    assert.equal(formatMuteUntil(Number.NaN), '--')
})

test('formatMuteUntil formats within the same year without year prefix', () => {
    const now = new Date(2026, 5, 1, 12, 0, 0)
    // 2026-07-13 11:01:00
    const muteDate = new Date(2026, 6, 13, 11, 1, 0)
    const muteUntil = Math.floor(muteDate.getTime() / 1000)

    assert.equal(formatMuteUntil(muteUntil, now), '07月13日 11:01')
})

test('formatMuteUntil formats across year with year prefix', () => {
    const now = new Date(2026, 11, 31, 23, 50, 0)
    // 2027-01-01 08:05:00 (next year)
    const nextYearDate = new Date(2027, 0, 1, 8, 5, 0)
    const nextYearUntil = Math.floor(nextYearDate.getTime() / 1000)

    assert.equal(formatMuteUntil(nextYearUntil, now), '2027年01月01日 08:05')

    // 2025-10-15 14:30:00 (past year)
    const pastYearDate = new Date(2025, 9, 15, 14, 30, 0)
    const pastYearUntil = Math.floor(pastYearDate.getTime() / 1000)

    assert.equal(formatMuteUntil(pastYearUntil, now), '2025年10月15日 14:30')
})

test('formatMuteUntil defaults to current date when now is omitted', () => {
    const currentYear = new Date().getFullYear()
    const sameYearDate = new Date(currentYear, 2, 5, 9, 30, 0)
    const sameYearUntil = Math.floor(sameYearDate.getTime() / 1000)
    assert.equal(formatMuteUntil(sameYearUntil), '03月05日 09:30')

    const nextYearDate = new Date(currentYear + 1, 0, 2, 15, 45, 0)
    const nextYearUntil = Math.floor(nextYearDate.getTime() / 1000)
    assert.equal(formatMuteUntil(nextYearUntil), `${currentYear + 1}年01月02日 15:45`)
})
