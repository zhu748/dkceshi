package tooldiagnostics

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func atomicJSON(path string, value any) (retErr error) {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return atomicData(path, data)
}

func atomicData(path string, data []byte) (retErr error) {
	var err error
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tool-diagnostic-*")
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			retErr = errors.Join(retErr, f.Close())
		}
		if err := os.Remove(f.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			retErr = errors.Join(retErr, err)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	err = f.Close()
	closed = true
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
