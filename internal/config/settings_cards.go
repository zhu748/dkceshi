package config

import (
	"fmt"
	"strings"
	"sync"
)

// SettingCardSpec declares one card of the settings page: a titled block that
// holds one or more registered settings.
//
// The card exists because the page's unit is a subject the user recognizes, not
// a config section: the two injection switches and their prompts are one card,
// while the config file keeps them as four sections. The registry is what keeps
// that grouping declared — moving a setting to another card is an edit in
// settings_defaults.go, not an edit in a web UI component.
//
// Grouping is optional. A setting no card names renders in a card of its own,
// so a new setting still needs nothing but its RegisterSetting entry.
type SettingCardSpec struct {
	// Key is the card's id. It names the card for the web UI and never reaches
	// the config file: the settings it holds are still stored under their own
	// keys.
	Key string
	// TitleKey and DescriptionKey are web UI locale keys for the card.
	TitleKey       string
	DescriptionKey string
	// Order places the card on the settings page, in the same sequence as
	// SectionForm.Order: a smaller value comes first, and two cards that declare
	// the same value keep their declaration order. Sections and setting cards
	// share one sequence, so a card's position is declared rather than inherited
	// from which registry happens to hold it.
	Order int
	// Settings are the registered settings that render in this card, in render
	// order. A setting must not be claimed by two cards, and a hidden setting
	// cannot be claimed at all — it has no controls to render.
	Settings []string
}

var (
	settingCardMu    sync.RWMutex
	settingCardSpecs = map[string]SettingCardSpec{}
	settingCardOrder []string
	settingCardOf    = map[string]string{}
)

// RegisterSettingCard declares a settings-page card. It panics on a malformed
// spec: every rejected shape below is a programming error that would otherwise
// surface as a setting rendered on the wrong card, on two cards, or nowhere.
func RegisterSettingCard(spec SettingCardSpec) {
	key := strings.TrimSpace(spec.Key)
	if key == "" {
		panic("config: setting card key must not be empty")
	}
	if strings.TrimSpace(spec.TitleKey) == "" {
		panic(fmt.Sprintf("config: setting card %q must declare a title locale key", key))
	}
	if len(spec.Settings) == 0 {
		panic(fmt.Sprintf("config: setting card %q names no settings", key))
	}
	settingCardMu.Lock()
	defer settingCardMu.Unlock()
	if _, dup := settingCardSpecs[key]; dup {
		panic(fmt.Sprintf("config: setting card %q is already registered", key))
	}
	// Validate the complete declaration before claiming any member. A rejected
	// card must not prevent a later valid card from using an earlier member.
	members := make(map[string]struct{}, len(spec.Settings))
	for _, member := range spec.Settings {
		name := strings.TrimSpace(member)
		if name == "" {
			panic(fmt.Sprintf("config: setting card %q names a setting with an empty key", key))
		}
		// A card naming an unknown setting would silently render nothing, and a
		// hidden one would put a config-file-only setting on the settings page.
		setting, ok := LookupSetting(name)
		if !ok {
			panic(fmt.Sprintf("config: setting card %q names %q, which is not a registered setting", key, name))
		}
		if setting.Hidden {
			panic(fmt.Sprintf("config: setting card %q names %q, which is a hidden setting", key, name))
		}
		if owner, taken := settingCardOf[name]; taken {
			panic(fmt.Sprintf("config: setting %q is claimed by both card %q and card %q", name, owner, key))
		}
		if _, duplicate := members[name]; duplicate {
			panic(fmt.Sprintf("config: setting %q is claimed by both card %q and card %q", name, key, key))
		}
		members[name] = struct{}{}
	}
	for name := range members {
		settingCardOf[name] = key
	}
	spec.Key = key
	settingCardSpecs[key] = spec
	settingCardOrder = append(settingCardOrder, key)
}

// SettingCards returns the declared cards in declaration order.
func SettingCards() []SettingCardSpec {
	settingCardMu.RLock()
	defer settingCardMu.RUnlock()
	out := make([]SettingCardSpec, 0, len(settingCardOrder))
	for _, key := range settingCardOrder {
		out = append(out, settingCardSpecs[key])
	}
	return out
}

// SettingCardOf returns the card a registered setting renders in. A setting no
// card names reports false and renders in a card of its own.
func SettingCardOf(key string) (SettingCardSpec, bool) {
	settingCardMu.RLock()
	defer settingCardMu.RUnlock()
	card, ok := settingCardSpecs[settingCardOf[key]]
	return card, ok
}
