import { useAccountEditor } from './useAccountEditor'
import { useAccountRuntime } from './useAccountRuntime'
import { useAccountTesting } from './useAccountTesting'
import { useCredentialActions } from './useCredentialActions'

// Keep the page's action API stable while each hook owns one set of operations.
export function useAccountActions(options) {
    const { loading: editorLoading, hideAccountEditor, ...editor } = useAccountEditor(options)
    const { loading: credentialLoading, offerManualToken, ...credentials } = useCredentialActions({ ...options, hideAccountEditor })
    const testing = useAccountTesting({ ...options, offerManualToken })
    const runtime = useAccountRuntime(options)

    return {
        ...editor,
        ...credentials,
        ...testing,
        ...runtime,
        loading: editorLoading || credentialLoading,
    }
}
