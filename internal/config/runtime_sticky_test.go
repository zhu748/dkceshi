package config

import (
	"encoding/json"
	"testing"
)

func TestStickySchedulingSettingPersistsAndClones(t *testing.T) {
	c := Config{}
	if err := ApplySections(&c, map[string]map[string]any{"runtime": {"account_sticky_enabled": true}}); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var restored Config
	if err = json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Runtime.AccountStickyEnabled == nil || !*restored.Runtime.AccountStickyEnabled {
		t.Fatal("sticky setting was lost")
	}
	clone := restored.Clone()
	*clone.Runtime.AccountStickyEnabled = false
	if !*restored.Runtime.AccountStickyEnabled {
		t.Fatal("snapshot shared mutable sticky toggle")
	}
	if err := ApplySections(&restored, map[string]map[string]any{"runtime": {"account_sticky_enabled": false}}); err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(restored)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "{}" || *restored.Runtime.AccountStickyEnabled {
		t.Fatal("turning sticky off was not persisted")
	}
}
