package cli

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
)

type shortCacheWriter struct{}

func (shortCacheWriter) Write(data []byte) (int, error) { return len(data) / 2, nil }
func TestCacheOutputShortWritesPropagate(t *testing.T) {
	t.Setenv("GORN_CACHE", filepath.Join(t.TempDir(), "absent"))
	for _, operation := range []string{"path", "list"} {
		err := CacheContext(context.Background(), CacheOpts{Operation: operation, JSON: true, Quiet: true, Stdout: shortCacheWriter{}, Stderr: io.Discard})
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatal(operation, err)
		}
	}
}
