package script_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gornkit/gorn/internal/script"
)

func TestHelperBundleCopiesProductionSourceAndTracksBytes(t *testing.T) {
	source := fstest.MapFS{
		"LICENSE":           {Data: []byte("license")},
		"sh/helper.go":      {Data: []byte("package sh\n")},
		"sh/os_windows.go":  {Data: []byte("//go:build windows\npackage sh\n")},
		"sh/helper_test.go": {Data: []byte("excluded")},
	}
	bundle, err := script.LoadHelper(source)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := bundle.WriteTo(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"LICENSE", "helper.go", "os_windows.go"} {
		original := source["sh/"+name]
		if name == "LICENSE" {
			original = source[name]
		}
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // G304: controlled bundle fixture names.
		if err != nil || !bytes.Equal(data, original.Data) {
			t.Fatalf("%s: %v %q", name, err, data)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "helper_test.go")); !os.IsNotExist(err) {
		t.Fatalf("test source was bundled: %v", err)
	}
	for _, name := range []string{"sh/helper.go", "LICENSE"} {
		original := source[name]
		source[name] = &fstest.MapFile{Data: append(bytes.Clone(original.Data), ' ')}
		changed, err := script.LoadHelper(source)
		if err != nil || changed.Digest() == bundle.Digest() {
			t.Fatalf("changed %s did not change identity: %v", name, err)
		}
		source[name] = original
	}
	again, err := script.LoadHelper(source)
	if err != nil || again.Digest() != bundle.Digest() {
		t.Fatalf("bundle identity is unstable: %v", err)
	}
}

func TestGenerateHelperModes(t *testing.T) {
	for _, tc := range []struct {
		name, header, body string
		bundled            bool
	}{
		{"preamble", "//gorn:preamble\n", "func main(){}", true},
		{"explicit", "", "import h \"github.com/gornkit/gorn/sh\";func main(){_ = h.PreambleSentinel}", true},
		{"external", "//gorn:require github.com/gornkit/gorn v1.0.0\n", "import \"github.com/gornkit/gorn/sh\";func main(){_ = sh.PreambleSentinel}", false},
		{"stdlib", "", "func main(){}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parseSource("helper.gorn", []byte(tc.header+"package main;"+tc.body))
			if err != nil {
				t.Fatal(err)
			}
			g, err := script.Generate(f)
			if err != nil || g.BundleHelper != tc.bundled {
				t.Fatalf("bundle=%v: %v", g, err)
			}
			if strings.Contains(string(g.ModGenerated), "replace "+script.HelperModule) != tc.bundled {
				t.Fatalf("unexpected helper replacement:\n%s", g.ModGenerated)
			}
		})
	}
	f, err := parseSource("reserved.gorn", []byte("//gorn:require github.com/gornkit/gorn/sh v1.0.0\npackage main;func main(){}"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := script.Generate(f); err == nil || !strings.Contains(err.Error(), "at line 1") || !strings.Contains(err.Error(), "require github.com/gornkit/gorn") {
		t.Fatalf("reserved child requirement: %v", err)
	}
}

func TestHelperBundleMissingAssets(t *testing.T) {
	for _, source := range []fs.FS{nil, fstest.MapFS{}, fstest.MapFS{"sh/helper_test.go": {}, "LICENSE": {}}} {
		if _, err := script.LoadHelper(source); err == nil {
			t.Fatal("accepted missing production source")
		}
	}
}
