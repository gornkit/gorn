//go:build (linux || darwin) && (amd64 || arm64)

//nolint:gosec // Controlled self-executable, executable copies, and temporary fixture paths.
package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func awaitLifecycleFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return data
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
	return nil
}

func TestInstalledCLICancelsNestedHelperGroup(t *testing.T) {
	bin := builtCLI(t)
	dir := t.TempDir()
	cacheRoot := filepath.Join(dir, "cache")
	t.Setenv("GORN_CACHE", cacheRoot)
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	path := writeScript(t, dir, "nested.gorn", `package main
import("fmt";"os";"os/exec";"os/signal";"syscall";"time";"github.com/gornkit/gorn/sh")
func main(){
 mode,heartbeat,groups:=os.Args[1],os.Args[2],os.Args[3]
 f,err:=os.OpenFile(groups,os.O_APPEND|os.O_CREATE|os.O_WRONLY,0600);if err!=nil{panic(err)};fmt.Fprintln(f,syscall.Getpgrp());f.Close()
 if mode=="grand"{signal.Ignore(syscall.SIGTERM);for{os.WriteFile(heartbeat,[]byte(fmt.Sprint(time.Now().UnixNano())),0600);time.Sleep(10*time.Millisecond)}}
 term:=make(chan os.Signal,1);signal.Notify(term,syscall.SIGTERM)
 if mode=="leader"{c:=exec.Command(os.Args[0],"inner",heartbeat,groups);c.Stdout,c.Stderr=os.Stdout,os.Stderr;if err:=c.Start();err!=nil{panic(err)}}else{if _,err:=sh.Cmd(os.Args[0],"grand",heartbeat,groups).Env("GORN_PROCESS_GROUP","self").Start();err!=nil{panic(err)}}
 <-term
}`)
	for _, mode := range []string{"cold", "warm", "bypass"} {
		t.Run(mode, func(t *testing.T) {
			heartbeat, groups := filepath.Join(dir, mode+"-heartbeat"), filepath.Join(dir, mode+"-groups")
			args := []string{"run", "-v"}
			if mode == "bypass" {
				args = append(args, "--no-cache")
			}
			cmd := exec.Command(bin, append(args, path, "leader", heartbeat, groups)...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Process.Kill() }()
			awaitLifecycleFile(t, heartbeat)
			groupText, err := os.ReadFile(groups)
			if err != nil {
				t.Fatal(err)
			}
			ids := strings.Fields(string(groupText))
			if len(ids) != 3 || ids[0] != ids[1] || ids[1] != ids[2] {
				t.Fatalf("nested groups = %q", groupText)
			}
			if _, err := strconv.Atoi(ids[0]); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Process.Signal(os.Interrupt); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				var exited *exec.ExitError
				if !errors.As(err, &exited) || exited.ExitCode() != 130 {
					t.Fatalf("interrupt status=%v\n%s", err, stderr.String())
				}
			case <-time.After(4 * time.Second):
				t.Fatal("runtime cancellation did not finish")
			}
			want := map[string]string{"cold": "cache: miss", "warm": "cache: hit", "bypass": "cache: bypass"}[mode]
			if !strings.Contains(stderr.String(), want) || strings.Contains(stderr.String(), "gorn: error") || stdout.Len() != 0 {
				t.Fatalf("output=%q\n%s", stdout.String(), stderr.String())
			}
			before, _ := os.ReadFile(heartbeat)
			time.Sleep(80 * time.Millisecond)
			after, _ := os.ReadFile(heartbeat)
			if string(before) != string(after) {
				t.Fatal("nested descendant survived")
			}
		})
	}
	entries, err := os.ReadDir(filepath.Join(cacheRoot, "tmp"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("staging remains: %v %v", entries, err)
	}
}

func TestInstalledCLITerminatesBlockedSourceInput(t *testing.T) {
	bin := builtCLI(t)
	cmd := exec.Command(bin, "run", "-")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	if _, err := input.Write([]byte("package main\n")); err != nil {
		t.Fatal(err)
	}
	// Keep the source stream open. Give the installed process time to install
	// its root signal handler; deterministic reader interruption has unit coverage.
	time.Sleep(300 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exited *exec.ExitError
		if !errors.As(err, &exited) || exited.ExitCode() != 143 || stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("blocked source: %v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked source did not stop")
	}
}
