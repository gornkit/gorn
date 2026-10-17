//nolint:gosec // File names are the fixed maintained-source list.
package main

import (
	"bytes"
	"io/fs"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedHelperMatchesMaintainedProductionFiles(t *testing.T) {
	entries, err := os.ReadDir("sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		name := "sh/" + entry.Name()
		embedded, err := fs.ReadFile(helperSources, name)
		if strings.HasSuffix(name, "_test.go") {
			if err == nil {
				t.Fatalf("test source embedded: %s", name)
			}
			continue
		}
		maintained, readErr := os.ReadFile(name) //nolint:gosec // G304: names come from the maintained sh directory.
		if err != nil || readErr != nil || !bytes.Equal(embedded, maintained) {
			t.Fatalf("missing or stale embedded production source %s: %v %v", name, err, readErr)
		}
	}
}

func TestPrivateProcessPrimitivesMatch(t *testing.T) {
	for _, suffix := range []string{"core", "posix", "linux", "darwin", "windows", "unsupported"} {
		name := "process_" + suffix + ".go"
		helper, err := os.ReadFile("sh/" + name)
		if err != nil {
			t.Fatal(err)
		}
		runner, err := os.ReadFile("internal/process/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bytes.Replace(helper, []byte("package sh\n"), []byte("package process\n"), 1), runner) {
			t.Fatalf("private process primitive differs: %s", name)
		}
	}
}
