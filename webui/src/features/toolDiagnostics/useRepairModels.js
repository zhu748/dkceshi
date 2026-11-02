import { useEffect, useRef, useState } from 'react'
import { createRepairModelResource, emptyRepairModelState } from './repairModelResource'

export function useRepairModels(authFetch) {
    const resource = useRef(createRepairModelResource()).current
    const [state, setState] = useState(emptyRepairModelState)
    useEffect(() => () => resource.cancel(), [resource])
    return {
        ...state,
        invalidate() { resource.cancel(); setState(emptyRepairModelState()) },
        load(draft) { return resource.load(authFetch, draft, patch => setState(previous => ({ ...previous, ...patch }))) },
    }
}
