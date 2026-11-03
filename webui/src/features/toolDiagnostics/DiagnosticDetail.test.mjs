import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { fileURLToPath } from 'node:url'
import React from 'react'
import TestRenderer from 'react-test-renderer'
import react from '@vitejs/plugin-react'
import { createServer } from 'vite'

const { act, create } = TestRenderer
let vite, DiagnosticDetail, I18nProvider, useI18n

before(async () => {
    vite = await createServer({
        root: fileURLToPath(new URL('../../../', import.meta.url)),
        configFile: false,
        plugins: [react()],
        server: { middlewareMode: true, hmr: false, watch: null },
        appType: 'custom', logLevel: 'error',
    })
    DiagnosticDetail = (await vite.ssrLoadModule('/src/features/toolDiagnostics/DiagnosticDetail.jsx')).default
    ;({ I18nProvider, useI18n } = await vite.ssrLoadModule('/src/i18n.jsx'))
})

after(async () => { await vite?.close() })

function record(result, source) {
    return {
        id: 'tool_render_test', status: result === 'success' ? 'direct_success' : result,
        surface: 'chat.completions', model: 'test-model', stream: true,
        pinned: result === 'failed', completed_at: result === 'running' ? undefined : 1020,
        delivery: result === 'success' ? 'written' : 'pending',
        attempts: [{
            number: 1, kind: 'initial', result, source, method: 'direct',
            started_at: 1000, completed_at: result === 'running' ? undefined : 1020,
            error: result === 'failed' ? 'Invalid tool arguments' : undefined,
            calls: result === 'running' ? [] : [{ ordinal: 1, name: 'read_file', parse: 'success', schema: 'passed', delivery: 'pending' }],
            steps: [{ stage: 'candidate', result: 'running' }],
        }],
    }
}

for (const lang of ['zh', 'en']) {
    test(`${lang} diagnostics renders the first request and later outcomes with real translations`, async t => {
        let i18n, renderer
        function Detail(props) {
            i18n = useI18n()
            return React.createElement(DiagnosticDetail, { ...props, t: i18n.t })
        }
        const view = value => React.createElement(I18nProvider, null, React.createElement(Detail, { record: value }))
        await act(async () => { renderer = create(view(null)) })
        t.after(() => act(() => renderer.unmount()))
        await act(async () => { i18n.setLang(lang) })
        assert.ok(JSON.stringify(renderer.toJSON()).includes(i18n.t('toolDiagnostics.selectRecord')))

        // The first attempt mounts only after a request produces a record.
        // Using the real provider catches dictionary objects rendered as text.
        for (const result of ['running', 'success', 'failed']) {
            for (const source of ['content', 'reasoning', undefined]) {
                await act(async () => { renderer.update(view(record(result, source))) })
                const output = JSON.stringify(renderer.toJSON())
                assert.ok(output.includes(lang === 'zh' ? '来源' : 'Source'))
                assert.ok(output.includes(source ? i18n.t(`toolDiagnostics.source.${source}`) : '—'))
                assert.ok(output.includes('test-model'))
                if (result !== 'running') assert.ok(output.includes('read_file'))
            }
        }
    })
}
