package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"
)

type androidProfile struct{ model, api string }

// androidProfilePresets is package-level: fillAndroidProfile runs on every
// upstream request via StableAndroidProfile (BaseHeadersForAuth) and used to
// rebuild this ~63-element slice on each call.
var androidProfilePresets = androidProfiles()

func androidProfiles() []androidProfile {
	out := []androidProfile{{"V2507A", "36"}, {"Pixel 6a", "36"}, {"Pixel 9a", "36"}}
	// Manufacturer model codes, with conservative Android API presets. See
	// docs/android-device-models.md for the corresponding vivo product pages.
	out = append(out,
		androidProfile{"V2241A", "33"},  // X90
		androidProfile{"V2242A", "33"},  // X90 Pro
		androidProfile{"V2227A", "33"},  // X90 Pro+
		androidProfile{"V2309A", "34"},  // X100
		androidProfile{"V2324A", "34"},  // X100 Pro
		androidProfile{"V2359A", "34"},  // X100s
		androidProfile{"V2324HA", "34"}, // X100s Pro
		androidProfile{"V2366GA", "34"}, // X100 Ultra
		androidProfile{"V2366HA", "34"}, // X100 Ultra
		androidProfile{"V2415A", "35"},  // X200
		androidProfile{"V2405A", "35"},  // X200 Pro
		androidProfile{"V2405DA", "35"}, // X200 Pro variant
		androidProfile{"V2419A", "35"},  // X200 Pro mini
		androidProfile{"V2454A", "35"},  // X200 Ultra
		androidProfile{"V2454DA", "35"}, // X200 Ultra variant
		androidProfile{"V2364A", "34"},  // S19
		androidProfile{"V2362A", "34"},  // S19 Pro
		androidProfile{"V2429A", "35"},  // S20
		androidProfile{"V2430A", "35"},  // S20 Pro
	)
	models := []string{"Pixel 6", "Pixel 6 Pro", "Pixel 7", "Pixel 7 Pro", "Pixel 7a", "Pixel Fold", "Pixel 8", "Pixel 8 Pro", "Pixel 8a", "Pixel 9", "Pixel 9 Pro", "Pixel 9 Pro XL", "Pixel 9 Pro Fold", "SM-S911B", "SM-S916B", "SM-S918B", "SM-S921B", "SM-S926B", "SM-S928B", "SM-S931B", "SM-S936B", "SM-S938B"}
	for _, model := range models {
		for _, api := range []string{"35", "36"} {
			out = append(out, androidProfile{model, api})
		}
	}
	return out
}

func fillAndroidProfile(acc Account, stable bool) Account {
	// Fast path: an already-complete profile is returned unchanged. Every
	// assignment below is guarded by "if <field> == empty", so for a complete
	// profile the seed hash, candidate scan and env lookups are pure overhead
	// on the per-request hot path (managed accounts are typically complete
	// because EnsureAccountDeviceProfile persists the fields once).
	if acc.DeviceModel != "" && acc.AndroidAPILevel != "" && acc.AndroidID != "" && acc.InstallationID != "" && acc.DeviceBrand != "" {
		return acc
	}
	seed := sha256.Sum256([]byte("ds2api/android/v1/" + acc.Identifier()))
	profiles := androidProfilePresets
	candidates := make([]androidProfile, 0, len(profiles))
	model := strings.TrimSpace(acc.DeviceModel)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_DEVICE_MODEL"))
	}
	api := strings.TrimSpace(acc.AndroidAPILevel)
	if api == "" {
		api = strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_ANDROID_API_LEVEL"))
	}
	for _, p := range profiles {
		if (model == "" || p.model == model) && (api == "" || p.api == api) {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		if model == "" {
			// An explicit API outside the preset list must not force one model.
			for _, p := range profiles {
				candidates = append(candidates, androidProfile{p.model, api})
			}
		} else {
			if api == "" {
				api = "36"
			}
			candidates = []androidProfile{{model, api}}
		}
	}
	idx := int(seed[0]) % len(candidates)
	if !stable {
		if n, err := rand.Int(rand.Reader, big.NewInt(int64(len(candidates)))); err == nil {
			idx = int(n.Int64())
		}
	}
	chosen := candidates[idx]
	if acc.DeviceModel == "" {
		acc.DeviceModel = chosen.model
	}
	if acc.AndroidAPILevel == "" {
		acc.AndroidAPILevel = chosen.api
	}
	legacyProfile := acc.AndroidID == "" && acc.DeviceBrand == ""
	if acc.DeviceBrand == "" {
		acc.DeviceBrand = AndroidBrandForModel(acc.DeviceModel)
	}
	if acc.AndroidID == "" {
		data := seed[:]
		if !stable {
			random := make([]byte, 32)
			if _, err := rand.Read(random); err == nil {
				data = random
			}
		}
		acc.AndroidID = hex.EncodeToString(data[:8])
	}
	if acc.InstallationID == "" {
		acc.InstallationID = strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_INSTALLATION_ID"))
		// Preserve the old local 44-character installation identity on upgrade.
		if raw, err := base64.StdEncoding.DecodeString(acc.DeviceID); legacyProfile && err == nil && len(raw) == 32 && acc.DeviceIDType == "" {
			acc.InstallationID = acc.DeviceID
		}
		if acc.InstallationID == "" {
			acc.InstallationID = AndroidInstallationID(acc)
		}
	}
	return acc
}

// StableAndroidProfile also supplies a repeatable device for token-only calls,
// which do not have a managed account profile to persist.
func StableAndroidProfile(acc Account) Account {
	return fillAndroidProfile(acc, true)
}

func (s *Store) EnsureAccountDeviceProfile(identifier string) (Account, bool) {
	// Every managed request passes through here and the overwhelmingly common
	// case is an already-complete profile; take the read lock for that path so
	// concurrent requests do not serialize on the store's write lock.
	s.mu.RLock()
	idx, ok := s.accMap[identifier]
	if !ok {
		s.mu.RUnlock()
		return Account{}, false
	}
	before := s.cfg.Accounts[idx]
	if before.DeviceModel != "" && before.AndroidAPILevel != "" && before.InstallationID != "" && before.AndroidID != "" && before.DeviceBrand != "" {
		acc := before.clone()
		s.mu.RUnlock()
		return acc, true
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	// Re-resolve under the write lock: the index may have moved between the
	// two acquisitions.
	idx, ok = s.accMap[identifier]
	if !ok {
		return Account{}, false
	}
	before = s.cfg.Accounts[idx]
	if before.DeviceModel != "" && before.AndroidAPILevel != "" && before.InstallationID != "" && before.AndroidID != "" && before.DeviceBrand != "" {
		return before.clone(), true
	}
	stable := IsVercel() || (s.fromEnv && !envWritebackEnabled())
	acc := fillAndroidProfile(before, stable)
	s.cfg.Accounts[idx] = acc
	if !stable {
		s.cfgDirty = true
		if err := s.saveLocked(); err != nil {
			Logger.Warn("[device_profile] persistence failed, using stable profile", "error", err)
			acc = fillAndroidProfile(before, true)
			s.cfg.Accounts[idx] = acc
		}
	}
	return acc.clone(), true
}

func ValidateAccountDeviceProfile(acc Account) error {
	if acc.Timezone != "" {
		if _, err := time.LoadLocation(acc.Timezone); err != nil {
			return errors.New("timezone must be an IANA time zone")
		}
	}
	if r := acc.PhotoCompressRatio; r != nil && (math.IsNaN(*r) || math.IsInf(*r, 0) || *r < 0 || *r > 1) {
		return errors.New("photo_compress_ratio must be between 0 and 1")
	}
	if f := acc.PictureCompressFormat; f != "" && f != "webp" && f != "jpg" {
		return errors.New("picture_compress_format must be webp or jpg")
	}
	if acc.AndroidAPILevel == "" {
		return nil
	}
	n, err := strconv.Atoi(acc.AndroidAPILevel)
	if err != nil || n < 1 || n > 999 {
		return errors.New("android_api_level must be an integer between 1 and 999")
	}
	return nil
}
