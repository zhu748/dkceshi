package config

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// AccountStorageKey avoids collisions from replacing punctuation in emails,
// and keeps account identifiers out of filenames on case-insensitive systems.
func AccountStorageKey(identifier string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(identifier)))
	return hex.EncodeToString(sum[:])
}

func SameAndroidDevice(a, b Account) bool {
	return a.Identifier() == b.Identifier() && a.DeviceModel == b.DeviceModel &&
		a.AndroidAPILevel == b.AndroidAPILevel && a.Locale == b.Locale &&
		a.Timezone == b.Timezone && a.AndroidID == b.AndroidID &&
		a.DeviceBrand == b.DeviceBrand && a.InstallationID == b.InstallationID
}

// InvalidateChangedDeviceIDs runs at the configuration transaction boundary,
// including config imports, instead of relying on individual admin forms.
func InvalidateChangedDeviceIDs(before Config, after *Config) {
	previous := make(map[string]Account, len(before.Accounts))
	for _, acc := range before.Accounts {
		previous[acc.Identifier()] = acc
	}
	for i, acc := range after.Accounts {
		old, exists := previous[acc.Identifier()]
		if exists && acc.DeviceIDType == DeviceIDTypeAuto && acc.DeviceID == old.DeviceID && !SameAndroidDevice(old, acc) {
			after.Accounts[i].DeviceID = ""
		}
	}
}
