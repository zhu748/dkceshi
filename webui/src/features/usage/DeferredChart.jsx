import { lazy, Suspense, useEffect, useRef, useState } from 'react'

const EChart = lazy(() => import('./EChart'))

// Reserve the chart's height while its code and canvas are deferred. Once a
// chart enters the viewport, retain it so scrolling does not recreate it.
export default function DeferredChart({ height = 272, loadingLabel, ...props }) {
    const ref = useRef(null)
    const [visible, setVisible] = useState(false)
    useEffect(() => {
        if (typeof IntersectionObserver === 'undefined') {
            setVisible(true)
            return undefined
        }
        let active = true
        const observer = new IntersectionObserver(entries => {
            if (active && entries.some(entry => entry.isIntersecting)) {
                active = false
                setVisible(true)
                observer.disconnect()
            }
        }, { rootMargin: '200px' })
        if (ref.current) observer.observe(ref.current)
        return () => { active = false; observer.disconnect() }
    }, [])
    const placeholder = <div role="status" style={{ height }}>{loadingLabel}</div>
    return <div ref={ref} style={{ minHeight: height }}>
        {visible ? <Suspense fallback={placeholder}><EChart {...props} height={height} /></Suspense> : placeholder}
    </div>
}
