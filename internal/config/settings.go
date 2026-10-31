package config

// A registered setting is a top-level config section that needs no hand-written
// plumbing. It is declared once, in settings_defaults.go, and everything else is
// derived from that declaration:
//
//   - the config-file codec (decode, encode and Clone),
//   - validation of the stored values,
//   - the admin settings read and write API,
//   - the web UI form, which the settings API describes field by field,
//     including which settings share a card and where that card sits on the
//     page (RegisterSettingCard in settings_cards.go).
//
// Adding or changing a setting is therefore:
//
//  1. a RegisterSetting entry in settings_defaults.go, and
//  2. the code that reads it, through Store.SettingBool / SettingInt /
//     SettingString.
//
// Putting it on an existing card is one more line in that card's declaration.
//
// No codec case, no accessor plumbing, no admin handler, no per-protocol wiring
// and no web UI edit. The JSON shape is the same one the hand-written sections
// use, so clients treat registered and unregistered sections uniformly:
//
//	{ "<key>": { "<field>": <value>, ... } }
//
// A section whose values need custom semantics (a nested object, an enum, a
// derived value, a per-key list) is not a registered setting; it is a
// registered section (sections.go), which owns the same plumbing for the
// hand-written shapes.
type FieldKind string

const (
	FieldBool   FieldKind = "bool"
	FieldInt    FieldKind = "int"
	FieldString FieldKind = "string"
	// FieldEnum and FieldGroup belong to a registered section's form (see
	// sections.go). A registered setting is a scalar by construction, so they
	// are rejected there and only rendered for a section field.
	FieldEnum  FieldKind = "enum"
	FieldGroup FieldKind = "group"
)

// FieldSpec declares one field of a registered setting.
type FieldSpec struct {
	// Name is the JSON key of the field inside the section, e.g. "enabled".
	Name string
	// Kind is the field's value type. It decides how the value is decoded from
	// the config file, how it is validated, and which input the web UI renders.
	Kind FieldKind
	// Default is what a reader gets when the config file does not mention the
	// field. Its Go type must match Kind. For an int field, a stored 0 also
	// reads as the default: zero means "unset" throughout this codebase, which
	// is the same rule ValidateIntRange applies.
	Default any
	// Min and Max bound an int field. They are enforced on write and on load.
	// On write the value the request carries is checked as it stands — the
	// request mentioning the field is what makes it set — so an explicit 0 is
	// rejected when Min is positive. On load a stored 0 skips the check, so a
	// field that is unset is never out of range.
	Min int
	Max int
	// Trim trims a string field on read, which is what every hand-written
	// accessor for a free-text field does.
	Trim bool
	// NonBlank marks a field whose *stored* value must not be blank. It applies
	// to the config file, not to the settings API: a stored value that is not
	// empty but trims to nothing is a typo and fails the load, while the write
	// path cannot tell "the user cleared the field" from "the user typed
	// spaces" and stores both as the unset (empty) value.
	NonBlank bool
	// ForceValue, when non-nil, is the value the field always reads as, whatever
	// the config says. It is how a retired feature keeps its kill switch in one
	// place: the config still round-trips the field and the settings API still
	// reports it, but a config file that predates the retirement cannot turn the
	// feature back on by itself. Its Go type must match Kind.
	ForceValue any
	// Multiline renders a textarea instead of a single-line input.
	Multiline bool
	// PrefillDefault makes the web UI start a text field with the section's
	// `default_<field>` text instead of an empty box, so editing it saves a copy
	// of the built-in text. A field without it shows that text as a placeholder
	// instead, which keeps "empty means use the built-in" true.
	PrefillDefault bool
	// LabelKey and HelpKey are web UI locale keys. Keeping the keys here rather
	// than the text keeps translation in the locale files, and lets the web UI
	// render a new field with no component change.
	LabelKey string
	HelpKey  string
	// Validate is an optional validator for the field value, called on write
	// (CoerceSettingField) and config load (ValidateRegisteredSettings).
	Validate func(val any) error
}

// SettingSpec declares a registered setting: one top-level config section and
// the fields it holds.
type SettingSpec struct {
	// Key is the config-file and settings-API key. It must not collide with a
	// hand-written config section.
	Key string
	// TitleKey and DescriptionKey are web UI locale keys naming the setting
	// itself. Inside a card that holds several settings (see SettingCardSpec)
	// the title labels the setting's switch and the description explains it; a
	// setting no card names is a card of its own and the two keys title that
	// card, while the switch falls back to its field's LabelKey so the card does
	// not say the same thing twice.
	TitleKey       string
	DescriptionKey string
	// Hidden keeps the section out of the web UI form. It is for settings that
	// are config-file only, so that registering one does not put a control on
	// the settings page.
	Hidden bool
	// Fields are the section's fields, in the order the web UI renders them.
	Fields []FieldSpec
}
