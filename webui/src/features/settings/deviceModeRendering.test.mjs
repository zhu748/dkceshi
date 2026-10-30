import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { fileURLToPath } from 'node:url'
import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import react from '@vitejs/plugin-react'
import { createServer } from 'vite'

let vite
let RegistrySettingsSection
let AccountsTable

before(async () => {
    vite = await createServer({
        root: fileURLToPath(new URL('../../../', import.meta.url)),
        configFile: false,
        plugins: [react()],
        server: { middlewareMode: true, hmr: false },
        appType: 'custom',
        logLevel: 'error',
    })
    RegistrySettingsSection = (await vite.ssrLoadModule('/src/features/settings/RegistrySettingsSection.jsx')).default
    AccountsTable = (await vite.ssrLoadModule('/src/features/account/AccountsTable.jsx')).default
})

after(async () => { await vite?.close() })

const sectionSpecs = [{
    key: 'runtime',
    title_key: 'settings.runtimeTitle',
    fields: [{
        name: 'device_id_mode',
        kind: 'enum',
        label_key: 'settings.deviceIdMode',
        help_key: 'settings.deviceIdModeHelp',
        options: [
            { value: 'manual', label_key: 'settings.deviceIdModeManual', description_key: 'settings.deviceIdModeManualDesc' },
            { value: 'auto', label_key: 'settings.deviceIdModeAuto', description_key: 'settings.deviceIdModeAutoDesc' },
        ],
    }],
}]

for (const mode of ['manual', 'auto']) {
    test(`actual settings component renders the ${mode} device ID selector`, () => {
        const html = renderToStaticMarkup(React.createElement(RegistrySettingsSection, {
            t: key => key,
            form: { runtime: { device_id_mode: mode } },
            setForm() {},
            specs: [],
            sectionSpecs,
        }))
        assert.match(html, /<select\b/)
        assert.match(html, /settings\.deviceIdMode</)
        assert.match(html, /settings\.deviceIdModeHelp/)
        assert.match(html, new RegExp(`value="${mode}" selected=""`))
        assert.match(html, new RegExp(`settings.deviceIdMode${mode === 'auto' ? 'Auto' : 'Manual'}Desc`))
    })
}

test('account toolbar keeps login IDs and Android profile management reachable', () => {
    const html = renderToStaticMarkup(React.createElement(AccountsTable, {
        t: key => key,
        accounts: [],
        proxies: [],
        totalAccounts: 0,
        page: 1,
        pageSize: 10,
        totalPages: 1,
        searchQuery: '',
        deviceIdMode: 'manual',
        deviceIdRemaining: 0,
        onOpenDeviceIds() {},
        onOpenAndroidDevices() {},
    }))
    assert.match(html, /accountManager\.manageDeviceId/)
    assert.match(html, /accountManager\.manageAndroidProfile/)
    assert.match(html, /accountManager\.deviceIdRemaining/)
})
