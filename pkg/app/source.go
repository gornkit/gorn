package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

// Error is a sentinel error type for invalid script sources.
type Error string

const (
	// ErrUnresolvedPath is returned when a Source is created with an empty path.
	ErrUnresolvedPath Error = "unresolved path"
	// ErrEmptyScript is returned when a Source is created with empty data.
	ErrEmptyScript Error = "script is empty"
)

// Error implements the error interface.
func (e Error) Error() string { return string(e) }

// maxSlugLen bounds the cosmetic slug suffix on cache dir names. It's a
// readability choice, kept well under the filesystem per-component limit
// (NAME_MAX is 255 bytes on common filesystems) so the full "<hash>-<slug>"
// name can never overflow it.
const maxSlugLen = 64

// Source is a validated gorn script: its resolved path, raw bytes, and the
// content-derived cache key. Construct one with New.
type Source struct {
	path          string
	data          []byte
	appKey        Key
	sourceKey     Key
	buildIdentity string
	slug          string
}

// New validates path and data and returns a Source with its Key and Slug
// computed. It rejects an empty path (ErrUnresolvedPath) or empty data
// (ErrEmptyScript). Paths are absolute and cleaned without resolving symlinks.
// The "-" convention uses a synthetic stdin.gorn path in the invocation cwd
// and a distinct cache identity from a file at that path.
func New(path string, data []byte) (*Source, error) {
	if path == "" {
		return nil, ErrUnresolvedPath
	}
	if len(data) == 0 {
		return nil, ErrEmptyScript
	}
	stdin := path == "-"
	if stdin {
		path = "stdin.gorn"
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve source path: %w", err)
	}
	identity := path
	if stdin {
		identity = "stdin\x00" + path
	}

	key := NewKey(identity, data)
	return &Source{
		sourceKey: key,
		path:      path,
		data:      bytes.Clone(data),
		appKey:    key,
		slug:      makeSlug(path),
	}, nil
}

// Path returns the resolved script path.
func (s *Source) Path() string { return s.path }

// Data returns an independent copy of the raw script bytes.
func (s *Source) Data() []byte { return bytes.Clone(s.data) }

// Key returns the content-derived cache key.
func (s *Source) Key() Key { return s.appKey }

func (s *Source) BuildIdentity() string { return s.buildIdentity }

// WithBuildIdentity returns a source sharing immutable bytes with a build key
// bound to the supplied dependency/profile identity. Rebinding is deterministic.
func (s *Source) WithBuildIdentity(identity string) *Source {
	clone := *s
	clone.buildIdentity = identity
	clone.appKey = clone.sourceKey
	if identity != "" {
		sum := sha256.Sum256([]byte(string(clone.sourceKey) + "\x00" + identity))
		clone.appKey = Key(hex.EncodeToString(sum[:]))
	}
	return &clone
}

// Slug returns a filesystem-safe, human-readable label derived from the script
// name (its base without the .gorn extension), for use as a cosmetic suffix on
// cache dir names. It is a pure function of the path, which is already part of
// Key, so it never changes a cache lookup's outcome. Computed once in New;
// may be empty.
func (s *Source) Slug() string { return s.slug }

func makeSlug(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".gorn")
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	slug := b.String()
	if len(slug) > maxSlugLen { // slug is ASCII, so a byte cut is a rune cut
		slug = slug[:maxSlugLen]
	}
	return slug
}
