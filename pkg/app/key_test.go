package app

import "testing"

// TestGenerateAppKeyDeterministicAndSensitive checks the exported key API:
// same inputs → same 64-hex key, and both source bytes and path affect it.
func TestGenerateAppKeyDeterministicAndSensitive(t *testing.T) {
	a := NewKey("p.gorn", []byte("x"))
	if a != NewKey("p.gorn", []byte("x")) {
		t.Fatal("same inputs produced different keys")
	}
	if len(a) != 64 {
		t.Fatalf("key len = %d, want 64 (hex sha256)", len(a))
	}
	if NewKey("p.gorn", []byte("y")) == a {
		t.Fatal("source bytes not reflected in key")
	}
	if NewKey("q.gorn", []byte("x")) == a {
		t.Fatal("source path not reflected in key")
	}
}

// TestGenerateAppKeyFieldBoundaries guards the length-prefixed encoding: two
// (path, source) pairs that concatenate to the same bytes must still differ.
func TestGenerateAppKeyFieldBoundaries(t *testing.T) {
	if NewKey("a", []byte("bc")) == NewKey("ab", []byte("c")) {
		t.Fatal("path/source boundary collision: encoding is ambiguous")
	}
}

// TestHashStructFieldsParticipate proves every keyInputs field feeds the hash,
// so a change to cacheVersion/goversion/goos/goarch (not just source) forces a
// cache miss. The higher-level NewKey tests can only vary source+path.
func TestHashStructFieldsParticipate(t *testing.T) {
	base := keyInputs{
		cacheVersion: 1,
		goversion:    "go1.26",
		sourcePath:   "/x",
		goos:         "linux",
		goarch:       "amd64",
		source:       []byte("s"),
	}
	h := hashStruct(base)

	mutations := map[string]func(*keyInputs){
		"cacheVersion": func(k *keyInputs) { k.cacheVersion = 2 },
		"goversion":    func(k *keyInputs) { k.goversion = "go1.27" },
		"sourcePath":   func(k *keyInputs) { k.sourcePath = "/y" },
		"goos":         func(k *keyInputs) { k.goos = "darwin" },
		"goarch":       func(k *keyInputs) { k.goarch = "arm64" },
		"source":       func(k *keyInputs) { k.source = []byte("t") },
	}
	for name, mutate := range mutations {
		cp := base
		mutate(&cp)
		if hashStruct(cp) == h {
			t.Errorf("changing %s did not change the hash", name)
		}
	}
}
