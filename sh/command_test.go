package sh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestCommandFixture(t *testing.T) {
	switch os.Getenv("GORN_COMMAND_FIXTURE") {
	case "echo":
		_ = json.NewEncoder(os.Stdout).Encode(os.Args[3:])
		os.Exit(0)
	case "wait":
		for {
			time.Sleep(time.Hour)
		}
	case "exit":
		os.Exit(7)
	case "inspect":
		data, _ := io.ReadAll(os.Stdin)
		cwd, _ := os.Getwd()
		_ = json.NewEncoder(os.Stdout).Encode([]string{cwd, os.Getenv("GORN_INSPECT_VALUE"), string(data)})
		_, _ = io.WriteString(os.Stderr, "diagnostic")
		os.Exit(0)
	}
}

func commandFixture(mode string, args ...string) Command {
	return Cmd(os.Args[0], append([]string{"-test.run=^TestCommandFixture$", "--"}, args...)...).Env("GORN_COMMAND_FIXTURE", mode)
}

func TestCmdExactArgumentsAndCopies(t *testing.T) {
	args := []string{"-test.run=^TestCommandFixture$", "--", "", "a b", "*", "$HOME", "a|b", "é", "\"quoted\"", "x\\y"}
	base := Cmd(os.Args[0], args...).Env("GORN_COMMAND_FIXTURE", "echo")
	args[2] = "changed"
	var out bytes.Buffer
	if got := base.Stdout(&out).Exec(); !got.OK() {
		t.Fatal(got.Error())
	}
	var actual []string
	if err := json.Unmarshal(out.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	want := []string{"", "a b", "*", "$HOME", "a|b", "é", "\"quoted\"", "x\\y"}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("args = %#v", actual)
	}
	if base.stdout != nil {
		t.Fatal("IO configuration mutated base")
	}
}

func TestCommandContextAndConcurrentWait(t *testing.T) {
	if _, err := commandFixture("echo").Context(nil).Start(); err == nil { //nolint:staticcheck // Verify explicit nil rejection.
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	base := commandFixture("wait")
	p, err := base.Context(ctx).Start()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	var wg sync.WaitGroup
	results := make(chan Result, 16)
	for range 16 {
		wg.Go(func() { results <- p.Wait() })
	}
	cancel()
	wg.Wait()
	close(results)
	for got := range results {
		if !errors.Is(got.Error(), context.Canceled) || got.OK() {
			t.Fatalf("Wait = %#v", got)
		}
	}
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	if base.contextSet {
		t.Fatal("Context mutated base")
	}
	if got := commandFixture("echo", "normal").Stdout(io.Discard).Exec(); !got.OK() {
		t.Fatal(got.Error())
	}
}

func TestCmdExecutablePathIOEnvDirAndFailures(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "with spaces 世界")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "program"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	in, err := os.Open(os.Args[0]) //nolint:gosec // Copy the running test executable into a private fixture.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o700) //nolint:gosec // Executable copy in a private fixture.
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("copy: %v %v", err, closeErr)
	}
	base := Cmd(path, "-test.run=^TestCommandFixture$", "--").Env("GORN_COMMAND_FIXTURE", "inspect").Env("GORN_INSPECT_VALUE", "original").Dir(dir)
	var stdout, stderr bytes.Buffer
	got := base.Env("GORN_INSPECT_VALUE", "changed").Stdin(bytes.NewBufferString("input")).Stdout(&stdout).Stderr(&stderr).Exec()
	if !got.OK() {
		t.Fatal(got.Error())
	}
	var data []string
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil || len(data) != 3 {
		t.Fatalf("output %q: %v", stdout.String(), err)
	}
	actual, err := os.Stat(data[0])
	want, statErr := os.Stat(dir)
	if err != nil || statErr != nil || !os.SameFile(actual, want) || data[1] != "changed" || data[2] != "input" || stderr.String() != "diagnostic" {
		t.Fatalf("IO/env/dir: %#v, stderr=%q", data, stderr.String())
	}
	if base.env["GORN_INSPECT_VALUE"] != "original" {
		t.Fatal("Env mutated base")
	}
	if got := commandFixture("exit").Exec(); got.Code() != 7 || got.Error() == nil {
		t.Fatalf("exit = %#v", got)
	}
	if got := Cmd(filepath.Join(dir, "absent")).Exec(); got.Code() != 1 || got.Error() == nil {
		t.Fatalf("startup = %#v", got)
	}
}
