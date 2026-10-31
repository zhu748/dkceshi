package config

import (
	"slices"
	"testing"
)

func TestRejectedSettingCardDoesNotClaimMembers(t *testing.T) {
	for _, reason := range []string{"unknown", "hidden", "duplicate"} {
		t.Run(reason, func(t *testing.T) {
			settingKey := "test_card_atomic_" + reason
			badCard, goodCard := settingKey+"_bad", settingKey+"_good"
			RegisterSetting(SettingSpec{
				Key: settingKey, TitleKey: "settings.test",
				Fields: []FieldSpec{{Name: "enabled", Kind: FieldBool, Default: false, LabelKey: "settings.test"}},
			})
			t.Cleanup(func() {
				settingCardMu.Lock()
				delete(settingCardSpecs, badCard)
				delete(settingCardSpecs, goodCard)
				delete(settingCardOf, settingKey)
				settingCardOrder = slices.DeleteFunc(settingCardOrder, func(key string) bool { return key == badCard || key == goodCard })
				settingCardMu.Unlock()
				settingMu.Lock()
				delete(settingSpecs, settingKey)
				delete(fieldSpecsFor, settingKey)
				settingOrder = slices.DeleteFunc(settingOrder, func(key string) bool { return key == settingKey })
				settingMu.Unlock()
			})

			invalidMember := "test_unknown_card_member"
			switch reason {
			case "hidden":
				invalidMember = "expert_prompt_segment"
			case "duplicate":
				invalidMember = settingKey
			}
			panicked := false
			func() {
				defer func() { panicked = recover() != nil }()
				RegisterSettingCard(SettingCardSpec{Key: badCard, TitleKey: "settings.test", Settings: []string{settingKey, invalidMember}})
			}()
			if !panicked {
				t.Fatal("invalid card must be rejected")
			}
			settingCardMu.RLock()
			_, claimed := settingCardOf[settingKey]
			settingCardMu.RUnlock()
			if claimed {
				t.Fatal("rejected card left a setting claimed; a later valid registration would fail")
			}
			RegisterSettingCard(SettingCardSpec{Key: goodCard, TitleKey: "settings.test", Settings: []string{settingKey}})
			if card, ok := SettingCardOf(settingKey); !ok || card.Key != goodCard {
				t.Fatal("valid registration after rejection must succeed")
			}
		})
	}
}
