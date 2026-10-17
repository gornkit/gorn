package script

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// HelperBundle owns the production helper source and its deterministic identity.
// The CLI supplies embedded files, keeping its import graph independent of sh.
type HelperBundle struct {
	files  map[string][]byte
	digest string
}

func LoadHelper(source fs.FS) (*HelperBundle, error) {
	if source == nil {
		return nil, fmt.Errorf("bundled helper source is unavailable")
	}
	entries, err := fs.ReadDir(source, "sh")
	if err != nil {
		return nil, fmt.Errorf("read bundled helper: %w", err)
	}
	files := map[string][]byte{
		"go.mod": fmt.Appendf(nil, "module %s\n\ngo %s\n", HelperModule, HelperGoVersion),
	}
	license, err := fs.ReadFile(source, "LICENSE")
	if err != nil {
		return nil, fmt.Errorf("read helper license: %w", err)
	}
	files["LICENSE"] = license
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if !fs.ValidPath(name) || strings.ContainsAny(name, `/\`) {
			return nil, fmt.Errorf("invalid bundled helper filename %q", name)
		}
		data, err := fs.ReadFile(source, "sh/"+name)
		if err != nil {
			return nil, fmt.Errorf("read helper %s: %w", name, err)
		}
		files[name] = data
	}
	if len(files) == 2 {
		return nil, fmt.Errorf("bundled helper has no production Go source")
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	hash := sha256.New()
	var size [8]byte
	for _, name := range names {
		for _, data := range [][]byte{[]byte(name), files[name]} {
			binary.LittleEndian.PutUint64(size[:], uint64(len(data)))
			hash.Write(size[:])
			hash.Write(data)
		}
	}
	return &HelperBundle{files: files, digest: hex.EncodeToString(hash.Sum(nil))}, nil
}

func (b *HelperBundle) Digest() string { return b.digest }

// WriteTo materializes a bundle inside a private generated-module directory.
func (b *HelperBundle) WriteTo(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil { //nolint:gosec // G703: caller supplies a private generated-module directory.
		return err
	}
	for name, data := range b.files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil { //nolint:gosec // G703: names come from validated embedded helper entries.
			return fmt.Errorf("write helper %s: %w", name, err)
		}
	}
	return nil
}
