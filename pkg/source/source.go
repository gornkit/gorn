package source

import (
	"path/filepath"
	"strings"
)

// SourceError is a sentinel error type for invalid script sources.
type SourceError string

const (
	// ErrUnresolvedPath is returned when a Source is created with an empty path.
	ErrUnresolvedPath SourceError = "unresolved path"
	// ErrEmptyScript is returned when a Source is created with empty data.
	ErrEmptyScript SourceError = "script is empty"
)

// Error implements the error interface.
func (e SourceError) Error() string { return string(e) }

// maxSlugLen bounds the cosmetic slug suffix on cache dir names. It's a
// readability choice, kept well under the filesystem per-component limit
// (NAME_MAX is 255 bytes on common filesystems) so the full "<hash>-<slug>"
// name can never overflow it.
const maxSlugLen = 64

// Source is a validated gorn script: its resolved path, raw bytes, and the
// content-derived cache key. Construct one with New.
type Source struct {
	path   string
	data   []byte
	appKey AppKey
	slug   string
}

// New validates path and data and returns a Source with its AppKey and Slug
// computed. It rejects an empty path (ErrUnresolvedPath) or empty data
// (ErrEmptyScript), and maps the "-" stdin convention to /dev/stdin.
func New(path string, data []byte) (*Source, error) {
	if path == "" {
		return nil, ErrUnresolvedPath
	}
	if len(data) == 0 {
		return nil, ErrEmptyScript
	}
	if path == "-" {
		path = "/dev/stdin"
	}

	return &Source{
		path:   path,
		data:   data,
		appKey: GenerateAppKey(path, data),
		slug:   makeSlug(path),
	}, nil
}

// Path returns the resolved script path.
func (s *Source) Path() string { return s.path }

// Data returns the raw script bytes.
func (s *Source) Data() []byte { return s.data }

// AppKey returns the content-derived cache key.
func (s *Source) AppKey() AppKey { return s.appKey }

// Slug returns a filesystem-safe, human-readable label derived from the script
// name (its base without the .gorn extension), for use as a cosmetic suffix on
// cache dir names. It is a pure function of the path, which is already part of
// AppKey, so it never changes a cache lookup's outcome. Computed once in New;
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
