package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Bump when derived detection rules change, even if the JSON shape does not.
const cacheSchema = 4

type cacheFile struct {
	Schema      int       `json:"schema"`
	Fingerprint string    `json:"fingerprint"`
	BaseFound   bool      `json:"base_found"`
	Packages    []Package `json:"packages"`
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "dnfscope", "packages.json"), nil
}

func readCache() (*cacheFile, error) {
	path, err := cachePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cf cacheFile
	if err := json.Unmarshal(b, &cf); err != nil {
		return nil, err
	}
	if cf.Schema != cacheSchema {
		return nil, errors.New("cache schema mismatch")
	}
	return &cf, nil
}

// writeCache stores the derived package list atomically; a failed write is
// never fatal, the app just re-scans next time.
func writeCache(cf *cacheFile) error {
	path, err := cachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(cf)
	if err != nil {
		return err
	}
	// Separate writers must not share a temporary file: concurrent scans could
	// otherwise rename a partially overwritten cache into place.
	f, err := os.CreateTemp(filepath.Dir(path), "packages-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
