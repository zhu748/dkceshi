import { useEffect, useState } from 'react'
import { createRepairTestResource } from './repairTestResource.js'

const empty = () => ({ loading: false, result: null, error: '' })
export function useRepairTest(authFetch) {
    const [resource] = useState(createRepairTestResource)
    const [state, setState] = useState(empty)
    useEffect(() => { setState(empty()); return resource.cancel }, [authFetch, resource])
    return {
        ...state,
        invalidate() { resource.cancel(); setState(empty()) },
        run(draft) { return resource.run(authFetch, draft, setState) },
    }
}
