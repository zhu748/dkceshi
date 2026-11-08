import assert from 'node:assert/strict'
import test from 'node:test'
import React from 'react'
import { create, act } from 'react-test-renderer'
import { isAccountCoolingDown, formatCooldownUntil } from './accountCooldown.js'
import { useAccountsData } from './useAccountsData.js'

test('cooldown expiry is independent of the enable switch and stale server flag', () => {
    const now = Date.now()
    assert.equal(isAccountCoolingDown({ enabled: true, cooldown_until: (now + 60000) / 1000 }, now), true)
    assert.equal(isAccountCoolingDown({ enabled: true, cooling_down: true, cooldown_until: (now - 1000) / 1000 }, now), false)
    assert.equal(isAccountCoolingDown({ enabled: false, cooldown_until: (now + 60000) / 1000 }, now), true)
    assert.equal(isAccountCoolingDown({ enabled: true, cooldown_until: 0 }, now), false)
    assert.equal(formatCooldownUntil('invalid'), '--')
    assert.match(formatCooldownUntil(Math.floor(now / 1000)), /:\d{2}$/)
})

test('background status polling preserves the current filter without a loading flash', async t => {
    let poll, state, renderer
    const urls = [], loading = []
    t.mock.method(globalThis, 'setInterval', fn => { poll = fn; return 1 })
    t.mock.method(globalThis, 'clearInterval', () => {})
    const apiFetch = async url => {
        urls.push(url)
        return { ok: true, json: async () => url.includes('/accounts')
            ? { items: [{ identifier: 'cool', enabled: true, cooling_down: true }], counts: { total: 1, enabled: 1, cooling_down: 1 }, page: 1 }
            : { available: 0 } }
    }
    function Harness() {
        state = useAccountsData({ apiFetch, t: key => key })
        loading.push(state.loadingAccounts)
        return null
    }
    try {
        await act(async () => { renderer = create(React.createElement(Harness)) })
        await act(async () => { state.handleFilterChange('cooling_down') })
        loading.length = 0
        await act(async () => { poll() })
        assert.equal(state.filterStatus, 'cooling_down')
        assert.equal(state.accountCounts.cooling_down, 1)
        assert.ok(urls.filter(url => url.includes('/accounts')).slice(-1)[0].includes('status=cooling_down'))
        assert.equal(loading.includes(true), false)
    } finally { if (renderer) await act(async () => renderer.unmount()) }
})
