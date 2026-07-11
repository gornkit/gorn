package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gornkit/gorn/pkg/app"
)

// manifestSchema versions the on-disk manifest. Bump it when the shape changes;
// Lookup treats a mismatched schema as a miss, so old entries rebuild.
const manifestSchema = 1

const manifestName = "manifest.json"

// Manifest is written into each app dir. It is both the cache-validity gate
// (Key + BinSHA256) and human-readable build metadata.
type Manifest struct {
	Schema    int    `json:"schema"`
	Key       string `json:"key"`
	BinSHA256 string `json:"bin_sha256"`
	GoVersion string `json:"go_version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	Source    string `json:"source"`
	BuiltAt   string `json:"built_at"`
}

func newManifest(s *app.Source, binSHA256 string) Manifest {
	return Manifest{
		Schema:    manifestSchema,
		Key:       string(s.Key()),
		BinSHA256: binSHA256,
		GoVersion: runtime.Version(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		Source:    s.Path(),
		BuiltAt:   time.Now().UTC().Format(time.RFC3339),
	}
}

// WriteManifest writes m as manifest.json into dir.
func WriteManifest(dir string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, manifestName), data, 0o600)
}

// LoadManifest reads and parses the manifest.json in dir.
func LoadManifest(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestName)) //nolint:gosec // G304: dir is a gorn-computed cache path, not user input.
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}
