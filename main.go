package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	arg "github.com/alexflint/go-arg"
	"github.com/gornkit/gorn/pkg/app"
)

var Version = "dev"

var subcommandNames = []string{"run", "build", "cache"}

type Common struct {
	Verbose bool `arg:"-v,--verbose" help:"Enable verbose diagnostics on stderr"`
}

type RunCmd struct {
	PrintGen  bool     `arg:"--print-gen" help:"Print generated go.mod and main.go, then exit (does not run)"`
	PrintMod  bool     `arg:"--print-mod" help:"Print the generated go.mod, then exit (does not run)"`
	PrintMain bool     `arg:"--print-main" help:"Print the generated main.go, then exit (does not run)"`
	NoCache   bool     `arg:"--no-cache" help:"Bypass any cached app output"`
	Script    string   `arg:"positional,required" placeholder:"script" help:"Script to run. Use - for stdin"`
	Args      []string `arg:"positional" placeholder:"args" help:"Arguments to pass to the script"`
}

type BuildCmd struct {
	// TODO
}

type CacheCmd struct {
	// TODO
}

type CLI struct {
	Run   *RunCmd   `arg:"subcommand:run" help:"Run a script. Default if no command specified"`
	Build *BuildCmd `arg:"subcommand:build" help:"Build a script"`
	Cache *CacheCmd `arg:"subcommand:cache" help:"Manage cache"`
}

func (CLI) Description() string {
	return "Go scripts you can run right now."
}

func (CLI) Version() string {
	return "gorn " + Version
}

func main() {
	if err := RunCLI(os.Args[1:], os.Stdout, os.Stderr, os.Exit); err != nil {
		fmt.Fprintf(os.Stderr, "gorn: error: %v\n", err)
		os.Exit(1)
	}
}

func RunCLI(args []string, stdout, stderr io.Writer, exit func(int)) error {
	var cli CLI
	var cmn Common

	parser, err := newParser(stdout, exit, &cli, &cmn)
	if err != nil {
		return err
	}
	err = parser.Parse(args)
	switch {
	case errors.Is(err, arg.ErrHelp): // found "--help" on command line
		parser.WriteHelp(stdout)
		exit(0)
	case errors.Is(err, arg.ErrVersion): // found "--version" on command line
		_, _ = fmt.Fprintln(stdout, cli.Version())
		exit(0)
	case err != nil:
		if startsWithSubcommand(args) {
			parser.WriteUsage(stdout)
			// RELEASE user facing error message
			return fmt.Errorf("failed to parse subcommand: %w", err)
		}
		runCmd := RunCmd{}
		runParser, err := newParser(stdout, exit, &runCmd, &cmn)
		if err != nil {
			// RELEASE user facing error message
			return fmt.Errorf("failed to create run parser: %w", err)
		}
		if err := runParser.Parse(args); err != nil {
			runParser.WriteUsage(stdout)
			// RELEASE user facing error message
			return fmt.Errorf("failed to parse run command: %w", err)
		}
		if err := runCmd.Run(&cmn, stdout, stderr); err != nil {
			runParser.WriteUsage(stdout)
			return err
		}
		return nil
	}

	return runSelected(parser, &cmn, stdout, stderr)
}

func startsWithSubcommand(args []string) bool {
	for _, arg := range args {
		if arg == "--verbose" || strings.HasPrefix(arg, "--verbose=") {
			continue
		}
		return slices.Contains(subcommandNames, arg)
	}
	return false
}

func newParser(out io.Writer, exit func(int), dest ...any) (*arg.Parser, error) {
	return arg.NewParser(arg.Config{
		Program: "gorn",
		Out:     out,
		Exit:    exit,
	}, dest...)
}

func runSelected(parser *arg.Parser, common *Common, stdout, stderr io.Writer) error {
	switch cmd := parser.Subcommand().(type) {
	case *RunCmd:
		return cmd.Run(common, stdout, stderr)
	case *BuildCmd:
		return cmd.Run(common, stdout, stderr)
	case *CacheCmd:
		return cmd.Run(common, stdout, stderr)
	default:
		parser.WriteHelp(stdout)
		return nil
	}
}

func (r *RunCmd) Run(common *Common, stdout, stderr io.Writer) error {
	script, err := scriptPath(r.Script)
	if err != nil {
		return err
	}
	return app.RunCmd(app.RunOpts{
		Stdout:     stdout,
		Stderr:     stderr,
		Verbose:    common.Verbose,
		PrintGen:   r.PrintGen,
		PrintMod:   r.PrintMod,
		PrintMain:  r.PrintMain,
		NoCache:    r.NoCache,
		ScriptPath: script,
		Args:       r.Args,
	})
}

func scriptPath(script string) (string, error) {
	if script == "-" {
		return script, nil
	}
	info, err := os.Stat(script)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", script)
	}
	return script, nil
}

func (b *BuildCmd) Run(common *Common, stdout, stderr io.Writer) error {
	return app.BuildCmd(app.BuildOpts{
		Verbose: common.Verbose,
	})
}

func (c *CacheCmd) Run(common *Common, stdout, stderr io.Writer) error {
	return app.CacheCmd(app.CacheOpts{
		Verbose: common.Verbose,
	})
}
