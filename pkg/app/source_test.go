package app

import (
	"errors"
	"testing"
)

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"hello.gorn":            "hello",
		"/a/b/deploy-tool.gorn": "deploy-tool",
		"we ird!.gorn":          "we_ird_",
		"-":                     "stdin", // stdin resolves to /dev/stdin
	}
	for path, want := range cases {
		s, err := New(path, []byte("body"))
		if err != nil {
			t.Fatalf("New(%q): %v", path, err)
		}
		if got := s.Slug(); got != want {
			t.Errorf("Slug(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestNewValidates(t *testing.T) {
	if _, err := New("", []byte("body")); !errors.Is(err, ErrUnresolvedPath) {
		t.Fatalf("empty path: err = %v, want ErrUnresolvedPath", err)
	}
	if _, err := New("x.gorn", nil); !errors.Is(err, ErrEmptyScript) {
		t.Fatalf("empty data: err = %v, want ErrEmptyScript", err)
	}

	s, err := New("-", []byte("body"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Path() != "/dev/stdin" {
		t.Fatalf("stdin path = %q, want /dev/stdin", s.Path())
	}
	if s.Key() == "" {
		t.Fatal("Key not computed")
	}
}
