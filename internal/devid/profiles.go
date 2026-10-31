package devid

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"ds2api/internal/config"
)

func ProfilePathFor(identifier string) string {
	return filepath.Join(config.DevidStateDir(), "profiles", config.AccountStorageKey(identifier)+".json")
}

func LegacyProfilePathFor(identifier string) string {
	return filepath.Join(config.DevidDir(), "profiles", config.SanitizeFilename(identifier)+".json")
}

// LegacyProfileUnique prevents migration/deletion of a legacy file that could
// also belong to another configured account.
func LegacyProfileUnique(identifier string, accounts []config.Account) bool {
	key := config.SanitizeFilename(identifier)
	for _, acc := range accounts {
		if acc.Identifier() != identifier && config.SanitizeFilename(acc.Identifier()) == key {
			return false
		}
	}
	return true
}

// ProfileUpdate isolates generator writes until the account still matches and
// the config store can commit the ID. Neither reset nor a failed generation
// destroys the existing archive.
type ProfileUpdate struct {
	Path, destination, legacy string
	accepted                  bool
}

func PrepareProfile(identifier string, accounts []config.Account, reset bool) (*ProfileUpdate, error) {
	p := &ProfileUpdate{destination: ProfilePathFor(identifier)}
	if LegacyProfileUnique(identifier, accounts) {
		p.legacy = LegacyProfilePathFor(identifier)
	}
	if err := os.MkdirAll(filepath.Dir(p.destination), 0o700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(filepath.Dir(p.destination), ".sdk-profile-*.json")
	if err != nil {
		return nil, err
	}
	p.Path = f.Name()
	if err := f.Close(); err != nil {
		p.Close()
		return nil, err
	}
	if err := os.Remove(p.Path); err != nil {
		return nil, err
	}
	if reset {
		return p, nil
	}
	source := p.destination
	if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
		source = p.legacy
	}
	if source == "" {
		return p, nil
	}
	body, _, err := readProfile(source)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(p.Path, body, 0o600); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

func readProfile(path string) ([]byte, os.FileMode, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() || info.Size() > 5<<20 {
		return nil, 0, fmt.Errorf("devid: invalid profile file: %s", path)
	}
	body, err := os.ReadFile(path)
	return body, info.Mode().Perm(), err
}

// Promote returns a rollback used if config persistence fails. The caller must
// hold its account lock through Promote, the config write, and Accept/Close.
func (p *ProfileUpdate) Promote() (func() error, error) {
	body, _, err := readProfile(p.Path)
	if err != nil {
		return nil, fmt.Errorf("devid: generator did not persist a profile: %w", err)
	}
	var archive struct {
		Version int                        `json:"version"`
		Profile map[string]json.RawMessage `json:"profile"`
	}
	if json.Unmarshal(body, &archive) != nil || archive.Version != 3 || archive.Profile == nil {
		return nil, errors.New("devid: generator persisted invalid profile JSON")
	}
	old, mode, err := readProfile(p.destination)
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.Rename(p.Path, p.destination); err != nil {
		return nil, err
	}
	return func() error {
		if !existed {
			return os.Remove(p.destination)
		}
		return writeProfileAtomic(p.destination, old, mode)
	}, nil
}

func (p *ProfileUpdate) Accept() { p.accepted = true }

func (p *ProfileUpdate) Close() {
	paths := []string{p.Path}
	if p.accepted && p.legacy != "" {
		paths = append(paths, p.legacy)
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			config.Logger.Warn("[devid] profile cleanup failed", "path", path, "error", err)
		}
	}
}

func writeProfileAtomic(path string, body []byte, mode os.FileMode) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".sdk-rollback-*.json")
	if err != nil {
		return err
	}
	defer func() {
		if removeErr := os.Remove(f.Name()); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			config.Logger.Warn("[devid] rollback cleanup failed", "error", removeErr)
		}
	}()
	if _, err = f.Write(body); err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// RemoveProfiles removes current archives and unambiguous legacy locations.
func RemoveProfiles(identifier string, accounts []config.Account) error {
	paths := []string{ProfilePathFor(identifier), config.DeviceProfilePath(identifier)}
	if LegacyProfileUnique(identifier, accounts) {
		paths = append(paths, LegacyProfilePathFor(identifier), filepath.Join(config.DeviceProfileDir(), config.SanitizeFilename(identifier)+".json"))
	}
	var result error
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}
	return result
}
