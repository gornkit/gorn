package sh

import (
	"io"
	"os"
	"os/exec"
	"strings"
)

// Command is a configured shell snippet that can be executed or started.
type Command struct {
	shell    ShellSpec
	script   string
	startDir string
	env      map[string]string
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
}

// Dir returns a copy that runs in path.
func (c Command) Dir(path string) Command {
	c.startDir = path
	return c
}

// Env returns a copy with key set to value for the command process.
func (c Command) Env(key, value string) Command {
	env := make(map[string]string, len(c.env)+1)
	for k, v := range c.env {
		env[k] = v
	}
	env[key] = value
	c.env = env
	return c
}

// Stdin returns a copy that reads standard input from r.
func (c Command) Stdin(r io.Reader) Command {
	c.stdin = r
	return c
}

// Stdout returns a copy that writes standard output to w.
func (c Command) Stdout(w io.Writer) Command {
	c.stdout = w
	return c
}

// Stderr returns a copy that writes standard error to w.
func (c Command) Stderr(w io.Writer) Command {
	c.stderr = w
	return c
}

// Exec starts the command and waits for it.
func (c Command) Exec() Result {
	p, err := c.Start()
	if err != nil {
		return Result{code: 1, err: err}
	}
	return p.Wait()
}

// Start starts the command without waiting for it.
func (c Command) Start() (*Process, error) {
	script := c.script
	if len(c.shell.setupLines) > 0 {
		lines := append([]string(nil), c.shell.setupLines...)
		script = strings.Join(append(lines, script), "\n")
	}

	args := append([]string(nil), c.shell.args...)
	args = append(args, script)
	cmd := exec.Command(c.shell.base, args...) //nolint:gosec // ponytail: shell helper intentionally runs caller-selected shell snippets.
	cmd.Dir = c.startDir
	cmd.Env = os.Environ()
	for k, v := range c.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	cmd.Stdin = c.stdin
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = c.stdout
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = c.stderr
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Process{cmd: cmd}, nil
}
