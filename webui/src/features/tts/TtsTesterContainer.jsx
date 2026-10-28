import { useCallback, useEffect, useRef, useState } from 'react'
import { Loader2, Play } from 'lucide-react'

import { useI18n } from '../../i18n'

const MAX_INPUT = 4096
const VOICES = [
    { id: 'mira', labelKey: 'tts.voiceMira' },
    { id: 'echo', labelKey: 'tts.voiceEcho' },
    { id: 'stella', labelKey: 'tts.voiceStella' },
    { id: 'tide', labelKey: 'tts.voiceTide' },
]

// decodeBase64URL turns the X-Ds2-Tts-Text-Base64 response header back into
// the text the model actually recited.
function decodeBase64URL(value) {
    if (!value) {
        return ''
    }
    try {
        const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
        const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4)
        const binary = atob(padded)
        const bytes = Uint8Array.from(binary, (ch) => ch.charCodeAt(0))
        return new TextDecoder().decode(bytes)
    } catch (_e) {
        return ''
    }
}

export default function TtsTesterContainer({ config, authFetch }) {
    const { t } = useI18n()
    const apiFetch = authFetch || fetch
    const configuredKeys = Array.isArray(config?.keys) ? config.keys : []

    const [enabled, setEnabled] = useState(true)
    const [settingsLoading, setSettingsLoading] = useState(true)
    const [voice, setVoice] = useState('mira')
    const [text, setText] = useState('')
    const [submitting, setSubmitting] = useState(false)
    const [error, setError] = useState('')
    const [audioURL, setAudioURL] = useState('')
    const [recitedText, setRecitedText] = useState('')
    const objectURLRef = useRef('')

    useEffect(() => {
        let disposed = false
        async function load() {
            try {
                const res = await apiFetch('/admin/settings')
                const data = await res.json()
                if (!res.ok) {
                    throw new Error(data?.detail || t('settings.loadFailed'))
                }
                if (!disposed) {
                    const tts = data?.tts || {}
                    setEnabled(tts.enabled !== false)
                }
            } catch (e) {
                if (!disposed) {
                    setError(e?.message || t('settings.loadFailed'))
                }
            } finally {
                if (!disposed) {
                    setSettingsLoading(false)
                }
            }
        }
        load()
        return () => {
            disposed = true
        }
    }, [apiFetch, t])

    useEffect(() => () => {
        if (objectURLRef.current) {
            URL.revokeObjectURL(objectURLRef.current)
        }
    }, [])

    const submit = useCallback(async () => {
        const input = text.trim()
        if (!input) {
            setError(t('tts.emptyText'))
            return
        }
        const key = (configuredKeys[0] || '').trim()
        if (!key) {
            setError(t('tts.noApiKey'))
            return
        }
        setSubmitting(true)
        setError('')
        try {
            const res = await fetch('/v1/audio/speech', {
                method: 'POST',
                headers: {
                    'Authorization': `Bearer ${key}`,
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ input, voice, response_format: 'wav' }),
            })
            if (!res.ok) {
                let message = res.statusText
                try {
                    const data = await res.json()
                    message = data?.error?.message || data?.detail || message
                } catch (_e) {
                    // Non-JSON error bodies keep the status text.
                }
                throw new Error(message)
            }
            const blob = await res.blob()
            const url = URL.createObjectURL(blob)
            if (objectURLRef.current) {
                URL.revokeObjectURL(objectURLRef.current)
            }
            objectURLRef.current = url
            setAudioURL(url)
            setRecitedText(decodeBase64URL(res.headers.get('X-Ds2-Tts-Text-Base64') || ''))
        } catch (e) {
            setError(t('tts.requestFailed', { message: e?.message || '' }))
        } finally {
            setSubmitting(false)
        }
    }, [configuredKeys, t, text, voice])

    const inputLength = Array.from(text).length
    const canSubmit = enabled && !settingsLoading && !submitting && inputLength > 0 && inputLength <= MAX_INPUT

    return (
        <div className="space-y-6">
            <div className="bg-card border border-border rounded-xl p-5 space-y-4">
                <label className="space-y-1.5 block">
                    <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                        {t('tts.voice')}
                    </span>
                    <select
                        value={voice}
                        onChange={(e) => setVoice(e.target.value)}
                        className="w-full bg-background border border-border rounded-lg px-3 py-2"
                    >
                        {VOICES.map((item) => (
                            <option key={item.id} value={item.id}>{t(item.labelKey)}</option>
                        ))}
                    </select>
                </label>

                <label className="space-y-1.5 block">
                    <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                        {t('tts.text')}
                    </span>
                    <textarea
                        value={text}
                        maxLength={MAX_INPUT}
                        rows={5}
                        onChange={(e) => setText(e.target.value)}
                        placeholder={t('tts.textPlaceholder')}
                        className="w-full bg-background border border-border rounded-lg px-3 py-2 resize-y"
                    />
                    <span className="block text-right text-xs text-muted-foreground">
                        {t('tts.charCount', { count: inputLength, max: MAX_INPUT })}
                    </span>
                </label>

                {error ? (
                    <div className="p-3 rounded-lg border border-destructive/30 bg-destructive/10 text-destructive text-sm">
                        {error}
                    </div>
                ) : null}

                <div className="flex flex-wrap items-center gap-3">
                    <button
                        type="button"
                        onClick={submit}
                        disabled={!canSubmit}
                        className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50"
                    >
                        {submitting ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                        {submitting ? t('tts.submitting') : t('tts.submit')}
                    </button>
                    {!enabled && !settingsLoading ? (
                        <span className="text-sm text-muted-foreground">{t('tts.disabled')}</span>
                    ) : null}
                </div>
            </div>

            {audioURL ? (
                <div className="bg-card border border-border rounded-xl p-5 space-y-3">
                    <h3 className="font-semibold">{t('tts.result')}</h3>
                    <audio controls src={audioURL} className="w-full" />
                    <p className="text-xs text-muted-foreground">{t('tts.playbackHint')}</p>
                    {recitedText ? (
                        <div className="space-y-1">
                            <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                                {t('tts.recitedText')}
                            </div>
                            <pre className="whitespace-pre-wrap break-words text-sm bg-background border border-border rounded-lg p-3">
                                {recitedText}
                            </pre>
                        </div>
                    ) : null}
                </div>
            ) : null}
        </div>
    )
}
