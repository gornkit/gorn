package cli

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/gornkit/gorn/pkg/script"
)

type failingWriter struct {
	remaining int
	err       error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

func TestPrintArtifactsWriteFailures(t *testing.T) {
	failure := errors.New("output closed")
	gen := &script.Generated{ModGenerated: []byte("module test\n"), MainGenerated: []byte("package main\n")}
	for _, flags := range []struct{ mod, main bool }{{true, false}, {false, true}, {true, true}} {
		writes := 1
		if flags.mod && flags.main {
			writes = 4
		}
		for after := range writes {
			out := &failingWriter{remaining: after, err: failure}
			err := printArtifacts(RunOpts{Stdout: out, PrintMod: flags.mod, PrintMain: flags.main}, gen)
			if !errors.Is(err, failure) {
				t.Fatalf("flags=%+v write=%d: %v", flags, after, err)
			}
		}
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestPrintArtifactsShortWrite(t *testing.T) {
	for _, both := range []bool{false, true} {
		err := printArtifacts(RunOpts{Stdout: shortWriter{}, PrintMain: true, PrintMod: both}, &script.Generated{MainGenerated: []byte("package main\n")})
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("short write (headers=%v): %v", both, err)
		}
	}
}

func TestRunInspectionPropagatesWriteFailure(t *testing.T) {
	failure := errors.New("output closed")
	err := Run(RunOpts{
		Stdout: &failingWriter{err: failure}, Stderr: &bytes.Buffer{},
		PrintMain: true, ScriptPath: "../../testdata/run_hello.gorn",
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Run error: %v", err)
	}
}
