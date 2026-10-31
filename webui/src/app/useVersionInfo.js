import { useEffect, useState } from 'react'

export function useVersionInfo(authFetch) {
    const [versionInfo, setVersionInfo] = useState(null)

    useEffect(() => {
        const controller = new AbortController()
        let timer
        let attempts = 0
        async function loadVersion() {
            try {
                const res = await authFetch('/admin/version', { signal: controller.signal })
                if (!res.ok) return
                const data = await res.json()
                if (controller.signal.aborted) return
                setVersionInfo(data)
                // The first response already contains the local version. A few
                // background reads pick up the shared, bounded update check.
                if (data.update_check_pending && ++attempts < 6) {
                    timer = setTimeout(loadVersion, 1000)
                }
            } catch { /* Update discovery must not affect the console. */ }
        }
        loadVersion()
        return () => {
            controller.abort()
            clearTimeout(timer)
        }
    }, [authFetch])

    return versionInfo
}
