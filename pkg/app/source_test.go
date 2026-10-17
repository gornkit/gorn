package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"hello.gorn":            "hello",
		"/a/b/deploy-tool.gorn": "deploy-tool",
		"we ird!.gorn":          "we_ird_",
		"-":                     "stdin", // synthetic source has a portable basename
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
	if !filepath.IsAbs(s.Path()) || filepath.Base(s.Path()) != "stdin.gorn" {
		t.Fatalf("stdin path = %q, want absolute synthetic stdin.gorn", s.Path())
	}
	if s.Key() == "" {
		t.Fatal("Key not computed")
	}
}

func TestSourceNormalizesLexicalIdentity(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("package main;func main(){}")
	relative, err := New(filepath.Join("tasks", "..", "hello 世界.gorn"), data)
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := New(filepath.Join(cwd, "hello 世界.gorn"), data)
	if err != nil {
		t.Fatal(err)
	}
	if relative.Path() != absolute.Path() || relative.Key() != absolute.Key() {
		t.Fatal("relative and absolute lexical identities differ")
	}
	other, err := New(filepath.Join(dir, "another", "hello 世界.gorn"), data)
	if err != nil || other.Key() == relative.Key() {
		t.Fatalf("same basename in a different directory must differ: %v", err)
	}
}

func TestSourcePreservesSymlinkSpelling(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "real.gorn")
	link := filepath.Join(dir, "alias.gorn")
	if err := os.WriteFile(path, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	a, err := New(path, []byte("source"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(link, []byte("source"))
	if err != nil {
		t.Fatal(err)
	}
	if b.Path() != link || a.Key() == b.Key() {
		t.Fatal("symlink spelling must remain a distinct identity")
	}
}

func TestStdinIdentityAndSourceOwnership(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	input := []byte("package main;func main(){}")
	stdin, err := New("-", input)
	if err != nil {
		t.Fatal(err)
	}
	file, err := New("stdin.gorn", input)
	if err != nil || stdin.Key() == file.Key() {
		t.Fatalf("stdin must not collide with a real synthetic-named file: %v", err)
	}
	key := stdin.Key()
	input[0] = 'x'
	copy := stdin.Data()
	copy[0] = 'y'
	if stdin.Key() != key || !bytes.HasPrefix(stdin.Data(), []byte("package main")) {
		t.Fatal("source ownership violated")
	}
	otherDir := t.TempDir()
	t.Chdir(otherDir)
	other, err := New("-", stdin.Data())
	if err != nil || other.Key() == stdin.Key() {
		t.Fatalf("stdin identity must include invocation cwd: %v", err)
	}
}
