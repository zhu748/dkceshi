export function parseJSONMap(raw, fieldName, t) {
    const text = String(raw || '').trim()
    if (!text) {
        return {}
    }
    let parsed
    try {
        parsed = JSON.parse(text)
    } catch (_e) {
        throw new Error(t('settings.invalidJsonField', { field: fieldName }))
    }
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error(t('settings.invalidJsonField', { field: fieldName }))
    }
    return parsed
}

// A declared int field's bounds, as the server sends them. A bound the
// declaration does not carry stays undefined and is not enforced.
function intFieldBounds(field) {
    return {
        min: Number.isFinite(field?.min) ? field.min : undefined,
        max: Number.isFinite(field?.max) ? field.max : undefined,
    }
}

// intFieldAccepts reports whether value may be sent for a declared int field.
// The rule comes from the declaration rather than from a hard-coded minimum, so
// a field whose bounds start at 0 (input_file.min_chars) still accepts it while
// a min-1 field (runtime.account_max_inflight) does not. A non-integer is
// rejected too: the backend decodes an int field with strconv.Atoi, so 5.5
// would fail the same way an out-of-range value does.
export function intFieldAccepts(field, value) {
    if (typeof value !== 'number' || !Number.isInteger(value)) {
        return false
    }
    const { min, max } = intFieldBounds(field)
    if (min !== undefined && value < min) {
        return false
    }
    if (max !== undefined && value > max) {
        return false
    }
    return true
}

// parseIntFieldInput turns the text of an int field's box into the value its
// control should report. It returns undefined for anything the declaration does
// not accept — an empty box, a half-typed or non-integer number, a value
// outside [min, max] — and the control then reports nothing, which leaves the
// field's previous value in place. Coercing an empty box to 0 instead would
// hand the backend a value it rejects (a section int the request mentions must
// be inside [min, max], and 0 is not "unset" there) and would cost the user
// every other edit in the same save.
export function parseIntFieldInput(field, text) {
    const raw = String(text ?? '').trim()
    if (!raw) {
        return undefined
    }
    const value = Number(raw)
    return intFieldAccepts(field, value) ? value : undefined
}

// fromServerSections rebuilds the registry-driven sections from the settings
// response: the registry describes the fields, and each section's values live
// under its own key — the same key the config file uses. The section object is
// copied as it arrived, so server-side extras (admin's has_password_hash) reach
// the components that show them.
export function fromServerSections(data, sectionSpecs) {
    const out = {}
    for (const spec of Array.isArray(sectionSpecs) ? sectionSpecs : []) {
        const key = String(spec?.key || '')
        if (!key) {
            continue
        }
        out[key] = data?.[key] && typeof data[key] === 'object' ? { ...data[key] } : {}
    }
    return out
}

export function fromServerForm(data, sectionSpecs) {
    return {
        ...fromServerSections(data, sectionSpecs),
        model_aliases_text: JSON.stringify(data.model_aliases || {}, null, 2),
        settings: fromServerSettings(data),
    }
}

// fromServerSettings rebuilds the registry-driven settings from the settings
// response: the registry describes the fields, and each setting's value lives
// under its own key in the same `{ <field>: <value> }` shape a hand-written
// section uses. Values the server did not report fall back to the declared
// default, so a field is never rendered as undefined.
//
// The section object is copied as it arrived, so server-side extras such as
// `default_prompt` reach the components that show them.
export function fromServerSettings(data) {
    const out = {}
    for (const spec of Array.isArray(data?.setting_specs) ? data.setting_specs : []) {
        const key = String(spec?.key || '')
        if (!key) {
            continue
        }
        const section = data?.[key] && typeof data[key] === 'object' ? { ...data[key] } : {}
        for (const field of Array.isArray(spec?.fields) ? spec.fields : []) {
            const name = String(field?.name || '')
            if (!name || name in section) {
                continue
            }
            section[name] = field?.default
        }
        out[key] = section
    }
    return out
}

// declaredFieldsByName indexes one declaration's fields, so the payload builder
// can ask what the values it holds are allowed to be.
function declaredFieldsByName(fields) {
    const index = new Map()
    for (const field of Array.isArray(fields) ? fields : []) {
        const name = String(field?.name || '')
        if (name) {
            index.set(name, field)
        }
    }
    return index
}

// declaredFieldsByKey indexes a spec list (setting_specs / section_specs) the
// same way, one level up: config key to declared fields by name.
function declaredFieldsByKey(specs) {
    const index = new Map()
    for (const spec of Array.isArray(specs) ? specs : []) {
        const key = String(spec?.key || '')
        if (key) {
            index.set(key, declaredFieldsByName(spec?.fields))
        }
    }
    return index
}

// sectionPayloadFields copies the values of one section — or of a nested group —
// for the settings request. A declared int field whose value the declaration
// does not accept is left out instead of sent: the backend answers 400 for such
// a value (a section int the request mentions must be inside [min, max]) and
// discards the whole save with it, while a field the request does not mention
// keeps its stored value. That is what keeps a value the page loaded from the
// backend (DS2API_ACCOUNT_MAX_QUEUE=0 arrives as 0 in a min-1 field) from being
// echoed back and blocking every other edit.
function sectionPayloadFields(section, fieldsByName) {
    const out = {}
    for (const [name, value] of Object.entries(section)) {
        // `default_<field>` entries are server-supplied display text, not
        // fields the user edits, so they never travel back.
        if (name.startsWith('default_')) {
            continue
        }
        const field = fieldsByName.get(name)
        if (field?.kind === 'int' && !intFieldAccepts(field, value)) {
            continue
        }
        if (field?.kind === 'group' && value && typeof value === 'object' && !Array.isArray(value)) {
            out[name] = sectionPayloadFields(value, declaredFieldsByName(field.fields))
            continue
        }
        out[name] = value
    }
    return out
}

export function toServerPayload(form, sectionSpecs = [], settingSpecs = []) {
    const payload = {}
    // Registered sections travel under their own config key, in the
    // `{ <field>: <value> }` shape the config file uses. Only the sections the
    // form knows about are sent, so a stale page never resets one the server
    // added; the server ignores any field it does not declare.
    for (const [key, fieldsByName] of declaredFieldsByKey(sectionSpecs)) {
        const section = form?.[key]
        if (!section || typeof section !== 'object') {
            continue
        }
        payload[key] = sectionPayloadFields(section, fieldsByName)
    }
    // Registry-driven settings travel the same way, one level down under
    // `settings`. A key the form holds but no spec describes (a stale page after
    // a setting was removed) travels unfiltered, exactly as it did before.
    const settingFields = declaredFieldsByKey(settingSpecs)
    for (const [key, section] of Object.entries(form.settings || {})) {
        if (!key || !section || typeof section !== 'object') {
            continue
        }
        payload[key] = sectionPayloadFields(section, settingFields.get(key) || new Map())
    }
    return payload
}
