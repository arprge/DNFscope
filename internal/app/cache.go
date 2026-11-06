package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Bump when derived detection rules change, even if the JSON shape does not.
const cacheSchema = 5

// Bump when the repo history record shape or derivation changes.
const repoHistorySchema = 1

type cacheFile struct {
	Schema      int       `json:"schema"`
	Fingerprint string    `json:"fingerprint"`
	BaseFound   bool      `json:"base_found"`
	Packages    []Package `json:"packages"`
}

// repoHistory remembers which repository installed each build, so scans only
// query dnf5 transactions newer than LastID.
type repoHistory struct {
	Schema int               `json:"schema"`
	LastID int               `json:"last_id"`
	Repos  map[string]string `json:"repos"`
}

func cacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "dnfscope"), nil
}

func cachePath() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "packages.json"), nil
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
	return writeJSONAtomic(path, cf)
}

// readRepoHistory returns nil when the record is missing or stale; callers then
// rebuild from the full dnf5 history.
func readRepoHistory() *repoHistory {
	dir, err := cacheDir()
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "repos.json"))
	if err != nil {
		return nil
	}
	var rh repoHistory
	if err := json.Unmarshal(b, &rh); err != nil || rh.Schema != repoHistorySchema || rh.Repos == nil {
		return nil
	}
	return &rh
}

func writeRepoHistory(rh *repoHistory) error {
	dir, err := cacheDir()
	if err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(dir, "repos.json"), rh)
}

func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	// Separate writers must not share a temporary file: concurrent scans could
	// otherwise rename a partially overwritten cache into place.
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-*.tmp")
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
