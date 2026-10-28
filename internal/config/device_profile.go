package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"math/big"
	"os"
	"strconv"
	"strings"
)

type androidProfile struct{ model, api string }

func androidProfiles() []androidProfile {
	out := []androidProfile{{"V2507A", "36"}, {"Pixel 6a", "36"}, {"Pixel 9a", "36"}}
	models := []string{"Pixel 6", "Pixel 6 Pro", "Pixel 7", "Pixel 7 Pro", "Pixel 7a", "Pixel Fold", "Pixel 8", "Pixel 8 Pro", "Pixel 8a", "Pixel 9", "Pixel 9 Pro", "Pixel 9 Pro XL", "Pixel 9 Pro Fold", "SM-S911B", "SM-S916B", "SM-S918B", "SM-S921B", "SM-S926B", "SM-S928B", "SM-S931B", "SM-S936B", "SM-S938B"}
	for _, model := range models {
		for _, api := range []string{"35", "36"} {
			out = append(out, androidProfile{model, api})
		}
	}
	return out
}

func fillAndroidProfile(acc Account, stable bool) Account {
	seed := sha256.Sum256([]byte("ds2api/android/v1/" + acc.Identifier()))
	profiles := androidProfiles()
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
			model = "V2507A"
		}
		if api == "" {
			api = "36"
		}
		candidates = []androidProfile{{model, api}}
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
	if acc.DeviceID == "" {
		acc.DeviceID = strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_DEVICE_ID"))
	}
	if acc.DeviceID == "" {
		data := seed[:]
		if !stable {
			random := make([]byte, 32)
			if _, err := rand.Read(random); err == nil {
				data = random
			}
		}
		acc.DeviceID = base64.StdEncoding.EncodeToString(data)
	}
	return acc
}

func (s *Store) EnsureAccountDeviceProfile(identifier string) (Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.accMap[identifier]
	if !ok {
		return Account{}, false
	}
	before := s.cfg.Accounts[idx]
	if before.DeviceModel != "" && before.AndroidAPILevel != "" && before.DeviceID != "" {
		return before, true
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
	return acc, true
}

func ValidateAccountDeviceProfile(acc Account) error {
	if acc.AndroidAPILevel == "" {
		return nil
	}
	n, err := strconv.Atoi(acc.AndroidAPILevel)
	if err != nil || n < 1 || n > 999 {
		return errors.New("android_api_level must be an integer between 1 and 999")
	}
	return nil
}
