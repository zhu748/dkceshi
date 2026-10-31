package config

import (
	"encoding/json"
)

// A registered section is a top-level config key that needs hand-written
// semantics: a nested object, an enum, a derived value, a record list. Those are
// the shapes a registered setting (settings.go) deliberately does not cover, and
// this registry is what keeps them from being wired one file at a time.
//
// A section is declared once, in its section_*.go file, and everything else is
// derived from that declaration:
//
//   - the config-file codec: decode, encode and Clone,
//   - validation and read-side normalization of the stored value,
//   - the config-import merge, for a section that declares a form,
//   - the admin settings read and write API,
//   - the web UI form, which the settings API describes field by field,
//     including nested groups and enums.
//
// Adding or changing a section is therefore:
//
//  1. a RegisterSection entry — its own file, holding the section's Go type
//     when the section is new, and
//  2. the code that reads it: a Store accessor, or the section's own admin
//     endpoints when its UI is bespoke (the account manager, the Vercel sync
//     page, the device-id pool dialog), plus the locale keys the declaration
//     names.
//
// The section's Go type and its behaviour stay hand-written; the registry owns
// the plumbing, not the semantics. A section that is a list of records with its
// own CRUD endpoints (accounts, api_keys, device_id_pool) registers for the
// codec and the lifecycle hooks and keeps those endpoints; a section the
// settings page edits declares a Form and gets its read, write and rendering for
// free.
type SectionSpec struct {
	// Key is the config-file key, e.g. "runtime". It must not collide with a
	// registered setting or another section.
	Key string
	// Form describes the settings-page card and the fields the settings API
	// reads and writes. A nil Form means the section is not on the settings
	// page at all: it is edited by its own endpoints, or only in the config
	// file.
	Form *SectionForm
	// Decode reads the section out of the config file. It is the only required
	// hook: a section the codec cannot read would be a silently ignored key.
	Decode func(c *Config, raw json.RawMessage) error
	// Encode returns the section's config-file value. ok=false omits the key
	// entirely, which is how an untouched section stays out of the file. A nil
	// Encode marks a key the codec still reads but never writes — a removed or
	// renamed section that is kept only so old config files do not fail.
	Encode func(c Config) (any, bool)
	// Clone deep-copies the section from src into dst. A nil Clone means the
	// section holds nothing that needs copying.
	Clone func(dst, src *Config)
	// Normalize canonicalizes the section in place on every load and every
	// store update.
	Normalize func(c *Config)
	// Validate checks the stored section. A config file that carries an invalid
	// value is rejected at load rather than at the point of use.
	Validate func(c Config) error
	// Merge folds an imported config's section into next. A nil Merge with a
	// Form means the fields the imported section carries are applied field by
	// field, exactly as a settings update applies them; a nil Merge without a
	// Form means the import leaves the section alone (a record list merges
	// through its own endpoints).
	Merge func(next, incoming *Config) error
}

// SectionForm describes a section on the settings page. The settings API reads
// it through each field's reader and writes it through each field's applier, so
// a section declares what it looks like once and both sides follow.
type SectionForm struct {
	// TitleKey and DescriptionKey are web UI locale keys for the section card.
	TitleKey       string
	DescriptionKey string
	// Order places the card on the settings page: a smaller value comes first,
	// and two sections that declare the same value keep their declaration order.
	// It exists because a card's position is a layout decision, not a property of
	// the file the declaration happens to live in — the settings page is the one
	// place where declaration order alone would make a filename load-bearing.
	Order int
	// Bespoke marks a section whose card is rendered by its own web UI
	// component (the security card's password flow). Its fields still travel
	// through the settings API, so its values round-trip like any other
	// section's; the generic renderer leaves the card alone.
	Bespoke bool
	// Fields are the section's fields, in the order the web UI renders them.
	Fields []SectionField
	// RemovedFields names fields this section used to declare and no longer
	// accepts, mapped to the field that replaced them. A settings update that
	// still carries one is rejected with an error naming the replacement: a
	// write the server cannot honour must say so, not return success having
	// changed nothing. It says nothing about the config file — a legacy key
	// there is read by the section's own Decode, so an old file keeps loading.
	RemovedFields map[string]string
	// Extra reports section-level values that are not form fields — UI metadata
	// such as admin.has_password_hash, which the settings API reports next to
	// the fields but no control edits.
	Extra func(s *Store) map[string]any
}

// SectionField declares one field of a section form. A leaf field (bool, int,
// string, enum) reads through Read and writes through Apply; a FieldGroup nests
// one level of declared fields and needs neither.
type SectionField struct {
	// Name is the JSON key of the field inside the section, e.g. "enabled".
	Name string
	// Kind is the field's value type. It decides how a value is coerced on
	// write and which input the web UI renders.
	Kind FieldKind
	// LabelKey and HelpKey are web UI locale keys. Keeping the keys here rather
	// than the text keeps translation in the locale files, and lets the web UI
	// render a new field with no component change. A FieldGroup may leave
	// LabelKey empty: a group that only binds a sub-feature's switch to the
	// settings it governs has no name of its own, and the web UI then renders
	// its fields straight into the card instead of printing a header that would
	// repeat the switch's own label.
	LabelKey string
	HelpKey  string
	// Min and Max bound an int field, on write and in the rendered input.
	Min int
	Max int
	// Multiline renders a textarea instead of a single-line input.
	Multiline bool
	// Options are the accepted values of a FieldEnum field, in render order.
	Options []SectionOption
	// RenamedValues maps a value this field used to accept onto the option that
	// replaced it. It exists so renaming an enum value stays a migration instead
	// of a breaking change: a write that still carries the old name — a stale
	// client, or a merge import of a config saved before the rename — is applied
	// as the new one. The old name is deliberately not an option, so it is never
	// rendered and the config is written back under the new name. Lookup uses the
	// same trimmed, lowercase form the enum check does; withRenamedValues
	// normalizes the keys for a declaration.
	RenamedValues map[string]string
	// Fields are the declared fields of a FieldGroup.
	Fields []SectionField
	// Read reports the field's effective value for the settings API: the value
	// the field behaves as, not necessarily the value the config file carries
	// (an environment fallback, a derived recommendation, a forced value).
	Read func(s *Store) any
	// Apply merges a value the request carried into the config. It is called
	// only when the request holds the field, so an absent field is left
	// untouched. It reports a user-facing error naming the field.
	Apply func(c *Config, raw any) error
}

// SectionOption is one accepted value of a FieldEnum field.
type SectionOption struct {
	Value string
	// LabelKey is the option's label in the select control.
	LabelKey string
	// DescriptionKey, when set, is shown below the control while this option is
	// selected.
	DescriptionKey string
	// WarningKey, when set, is shown as a warning while this option is
	// selected.
	WarningKey string
}
