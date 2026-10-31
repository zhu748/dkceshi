import { useCallback, useEffect, useRef } from 'react'
import { createChatRequest } from './chatRequest.js'

export function useChatStreamClient(options) {
    const client = useRef(null)
    if (!client.current) client.current = createChatRequest(options)
    useEffect(() => () => client.current.cancel(), [])
    const runTest = useCallback(() => client.current.run(options), [options])
    const stopGeneration = useCallback(() => {
        if (client.current.stop()) options.onMessage('info', options.t('messages.generationStopped'))
    }, [options])
    return { runTest, stopGeneration }
}
