package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	manifestSchema = 1
	manifestName   = "manifest.json"
)

// Manifest is the JSON document written alongside each cached binary.
// It stores enough information for CachedBin to perform an exact key check,
// and for diagnostics or tooling to inspect what is cached.
type Manifest struct {
	Schema    int    `json:"schema"`
	AppKey    string `json:"app_key"`
	GoVersion string `json:"go_version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	Source    string `json:"source"`
	BuiltAt   string `json:"built_at"`
}

// newManifest builds a Manifest for appKey, stamped with the current toolchain
// and time. source is the human-readable script path or identifier.
func newManifest(appKey AppKey, source string) Manifest {
	return Manifest{
		Schema:    manifestSchema,
		AppKey:    string(appKey),
		GoVersion: runtime.Version(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		Source:    source,
		BuiltAt:   time.Now().UTC().Format(time.RFC3339),
	}
}

// WriteManifest marshals m to JSON and writes it to dir/manifest.json with
// 0o600 permissions.
func WriteManifest(dir string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, manifestName)
	return os.WriteFile(path, data, 0o600) //nolint:gosec // intentionally 0600 for cache files
}

// LoadManifest reads and unmarshals the manifest from dir/manifest.json.
// It returns an error if the file is missing or cannot be decoded.
func LoadManifest(dir string) (Manifest, error) {
	path := filepath.Join(dir, manifestName)
	data, err := os.ReadFile(path) //nolint:gosec // dir comes from our own cache root
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}
