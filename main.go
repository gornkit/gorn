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
	Verbose bool `help:"Enable verbose output"`
}

type RunCmd struct {
	PrintGen bool     `arg:"--print-gen" help:"Print generated output"`
	Script   string   `arg:"positional,required" placeholder:"script" help:"Script to run. Use - for stdin"`
	Args     []string `arg:"positional" placeholder:"args" help:"Arguments to pass to the script"`
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
	if err := RunCLI(os.Args[1:], os.Stdout, os.Exit); err != nil {
		fmt.Fprintf(os.Stderr, "gorn: error: %v\n", err)
		os.Exit(1)
	}
}

func RunCLI(args []string, out io.Writer, exit func(int)) error {
	var cli CLI
	var cmn Common

	parser, err := newParser(out, exit, &cli, &cmn)
	if err != nil {
		return err
	}
	err = parser.Parse(args)
	switch {
	case errors.Is(err, arg.ErrHelp): // found "--help" on command line
		parser.WriteHelp(out)
		exit(0)
	case errors.Is(err, arg.ErrVersion): // found "--version" on command line
		_, _ = fmt.Fprintln(out, cli.Version())
		exit(0)
	case err != nil:
		if startsWithSubcommand(args) {
			parser.WriteUsage(out)
			// RELEASE user facing error message
			return fmt.Errorf("failed to parse subcommand: %w", err)
		}
		runCmd := RunCmd{}
		runParser, err := newParser(out, exit, &runCmd, &cmn)
		if err != nil {
			// RELEASE user facing error message
			return fmt.Errorf("failed to create run parser: %w", err)
		}
		if err := runParser.Parse(args); err != nil {
			runParser.WriteUsage(out)
			// RELEASE user facing error message
			return fmt.Errorf("failed to parse run command: %w", err)
		}
		if err := runCmd.Run(&cmn); err != nil {
			runParser.WriteUsage(out)
			return err
		}
		return nil
	}

	return runSelected(parser, &cmn, out)
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

func runSelected(parser *arg.Parser, common *Common, out io.Writer) error {
	switch cmd := parser.Subcommand().(type) {
	case *RunCmd:
		return cmd.Run(common)
	case *BuildCmd:
		return cmd.Run(common)
	case *CacheCmd:
		return cmd.Run(common)
	default:
		parser.WriteHelp(out)
		return nil
	}
}

func (r *RunCmd) Run(common *Common) error {
	script, err := scriptPath(r.Script)
	if err != nil {
		return err
	}
	return app.RunCmd(app.RunOpts{
		Verbose:  common.Verbose,
		PrintGen: r.PrintGen,
		Script:   script,
		Args:     r.Args,
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

func (b *BuildCmd) Run(common *Common) error {
	return app.BuildCmd(app.BuildOpts{
		Verbose: common.Verbose,
	})
}

func (c *CacheCmd) Run(common *Common) error {
	return app.CacheCmd(app.CacheOpts{
		Verbose: common.Verbose,
	})
}
