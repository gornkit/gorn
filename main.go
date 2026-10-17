package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"
	"time"

	arg "github.com/alexflint/go-arg"
	"github.com/gornkit/gorn/internal/cli"
)

//go:embed sh/command.go sh/doc.go sh/pkg.go sh/process.go sh/process_core.go sh/process_posix.go sh/process_linux.go sh/process_darwin.go sh/process_windows.go sh/process_unsupported.go sh/process_marker.go sh/result.go sh/shellspec.go sh/shell_command_other.go sh/shell_command_windows.go LICENSE
var helperSources embed.FS

var Version string

var subcommandNames = []string{"run", "build", "cache"}

type Common struct {
	Verbose bool `arg:"-v,--verbose" help:"Enable verbose diagnostics on stderr"`
	Quiet   bool `arg:"-q,--quiet" help:"Suppress phase and heartbeat progress; retain errors and script stderr"`
	ctx     context.Context
	input   cli.Input
}

type runArgs struct {
	PrintMod  bool     `arg:"--print-mod" help:"Print the generated go.mod, then exit (does not run)"`
	PrintMain bool     `arg:"--print-main" help:"Print the generated main.go, then exit (does not run)"`
	NoCache   bool     `arg:"--no-cache" help:"Bypass any cached app output"`
	Script    string   `arg:"positional,required" placeholder:"script" help:"Script to run. Use - for stdin"`
	Args      []string `arg:"positional" placeholder:"args" help:"Arguments to pass to the script"`
}

func (runArgs) Version() string { return (CLI{}).Version() }

type buildArgs struct {
	Script  string `arg:"positional,required" placeholder:"script" help:"Script to compile without executing. Use - for stdin"`
	Output  string `arg:"-o,--output,required" placeholder:"path" help:"Output executable path (parent must exist)"`
	Force   bool   `arg:"--force" help:"Replace an existing regular output file"`
	NoCache bool   `arg:"--no-cache" help:"Build fresh and bypass cached app output"`
}

type cacheArgs struct {
	Path  *cachePathArgs  `arg:"subcommand:path" help:"Print the selected cache root without creating it"`
	List  *cacheListArgs  `arg:"subcommand:list" help:"Inspect entries, provenance and preserved data"`
	Clean *cacheCleanArgs `arg:"subcommand:clean" help:"Delete selected owned data, skipping active entries"`
	Prune *cachePruneArgs `arg:"subcommand:prune" help:"Prune current owned entries by build age or byte budget"`
}
type (
	cachePathArgs struct {
		JSON bool `arg:"--json" help:"Emit versioned JSON"`
	}
	cacheListArgs struct {
		JSON bool `arg:"--json" help:"Emit versioned JSON"`
	}
	cacheCleanArgs struct {
		KeyPrefix string `arg:"positional" placeholder:"key-prefix" help:"One unambiguous lowercase hexadecimal key prefix"`
		All       bool   `arg:"--all" help:"Select owned entries, abandoned stages and quarantine"`
		Yes       bool   `arg:"--yes" help:"Authorize deletion without a prompt"`
		DryRun    bool   `arg:"--dry-run" help:"Preview selection without deleting or reserving activity"`
		JSON      bool   `arg:"--json" help:"Emit versioned JSON, including partial results"`
	}
)

type cachePruneArgs struct {
	MaxAge   *time.Duration `arg:"--max-age" placeholder:"duration" help:"Remove entries older than this build age (for example 24h)"`
	MaxBytes *int64         `arg:"--max-bytes" placeholder:"bytes" help:"Evict oldest entries to meet a nonnegative logical-byte budget"`
	Yes      bool           `arg:"--yes" help:"Authorize deletion without a prompt"`
	DryRun   bool           `arg:"--dry-run" help:"Preview selection without deleting or reserving activity"`
	JSON     bool           `arg:"--json" help:"Emit versioned JSON, including partial results"`
}

type CLI struct {
	Run   *runArgs   `arg:"subcommand:run" help:"Run a script. Default if no command specified"`
	Build *buildArgs `arg:"subcommand:build" help:"Build a script"`
	Cache *cacheArgs `arg:"subcommand:cache" help:"Manage cache"`
}

func (CLI) Description() string {
	return "Go scripts you can run right now."
}

func (CLI) Version() string {
	info, _ := debug.ReadBuildInfo()
	moduleVersion := ""
	localVCS := false
	if info != nil {
		moduleVersion = info.Main.Version
		localVCS = slices.ContainsFunc(info.Settings, func(setting debug.BuildSetting) bool { return setting.Key == "vcs.revision" })
	}
	return "gorn " + selectedVersion(Version, moduleVersion, localVCS)
}

// An explicit stamp wins. Source installations carry their selected module
// version; ordinary local builds keep the established dev label.
func selectedVersion(stamp, moduleVersion string, localVCS bool) string {
	if stamp != "" {
		return stamp
	}
	if !localVCS && moduleVersion != "" && moduleVersion != "(devel)" {
		return moduleVersion
	}
	return "dev"
}

func main() {
	os.Exit(mainExitCode())
}

func mainExitCode() int {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupts)
	go func() {
		select {
		case sig := <-interrupts:
			code := 130
			if sig == syscall.SIGTERM {
				code = 143
			}
			cancel(&cli.StatusError{Code: code, Err: context.Canceled, Silent: true})
		case <-ctx.Done():
		}
	}()
	err := RunCLIContext(ctx, os.Args[1:], cli.Input{Reader: os.Stdin, CollectSource: collectRootSource}, os.Stdout, os.Stderr, os.Exit)
	if ctx.Err() != nil {
		err = errors.Join(context.Cause(ctx), err)
	}
	if err != nil {
		var status *cli.StatusError
		if !errors.As(err, &status) || !status.Silent {
			fmt.Fprintf(os.Stderr, "gorn: error: %v\n", err)
		}
		return cli.ExitStatus(err)
	}
	return 0
}

// collectRootSource belongs only to the exit-on-cancellation root process.
// Inherited blocking stdin cannot reliably be interrupted by os.File.Close.
// Cancellation happens before any cache/build/runtime resource is acquired;
// main exits immediately and the OS retires this otherwise blocked read thread.
// Embedding entry points must use their explicit interrupt/join ownership path.
func collectRootSource(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() { data, err := io.ReadAll(os.Stdin); done <- result{data, err} }()
	select {
	case got := <-done:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return got.data, got.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func RunCLI(args []string, stdout, stderr io.Writer, exit func(int)) error {
	return RunCLIContext(context.Background(), args, cli.Input{Reader: os.Stdin}, stdout, stderr, exit)
}

// RunCLIContext embeds CLI dispatch with explicit source-input interruption.
// It installs no process-global signal handlers and never closes caller IO.
func RunCLIContext(ctx context.Context, args []string, input cli.Input, stdout, stderr io.Writer, exit func(int)) error {
	if ctx == nil {
		return fmt.Errorf("nil CLI context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var cli CLI
	cmn := Common{ctx: ctx, input: input}

	parser, err := newParser(stdout, exit, &cli, &cmn)
	if err != nil {
		return err
	}
	err = parser.Parse(args)
	switch {
	case errors.Is(err, arg.ErrHelp): // found "--help" on command line
		parser.WriteHelp(stdout)
		exit(0)
		return nil
	case errors.Is(err, arg.ErrVersion): // found "--version" on command line
		_, _ = fmt.Fprintln(stdout, cli.Version())
		exit(0)
		return nil
	case err != nil:
		if startsWithSubcommand(args) {
			parser.WriteUsage(stderr)
			// RELEASE user facing error message
			return usageError(fmt.Errorf("failed to parse subcommand: %w", err))
		}
		runCmd := runArgs{}
		runParser, err := newParser(stdout, exit, &runCmd, &cmn)
		if err != nil {
			// RELEASE user facing error message
			return fmt.Errorf("failed to create run parser: %w", err)
		}
		err = runParser.Parse(args)
		switch {
		case errors.Is(err, arg.ErrHelp):
			runParser.WriteHelp(stdout)
			exit(0)
			return nil
		case errors.Is(err, arg.ErrVersion):
			_, _ = fmt.Fprintln(stdout, (CLI{}).Version())
			exit(0)
			return nil
		case err != nil:
			runParser.WriteUsage(stderr)
			return usageError(fmt.Errorf("failed to parse run command: %w", err))
		}
		return runCmd.Run(&cmn, stdout, stderr)
	}

	return runSelected(parser, &cmn, stdout, stderr)
}

func usageError(err error) error {
	return &cli.StatusError{Code: 2, Err: err}
}

func startsWithSubcommand(args []string) bool {
	for _, arg := range args {
		if arg == "-v" || arg == "--verbose" || strings.HasPrefix(arg, "--verbose=") || strings.HasPrefix(arg, "-v=") ||
			arg == "-q" || arg == "--quiet" || strings.HasPrefix(arg, "--quiet=") || strings.HasPrefix(arg, "-q=") {
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
	case *runArgs:
		return cmd.Run(common, stdout, stderr)
	case *buildArgs:
		return cmd.Run(common, stdout, stderr)
	case *cacheArgs:
		parser.WriteHelp(stdout)
		return nil
	case *cachePathArgs:
		return cmd.Run(common, stdout, stderr)
	case *cacheListArgs:
		return cmd.Run(common, stdout, stderr)
	case *cacheCleanArgs:
		return cmd.Run(common, stdout, stderr)
	case *cachePruneArgs:
		return cmd.Run(common, stdout, stderr)
	default:
		parser.WriteHelp(stdout)
		return nil
	}
}

func (r *runArgs) Run(common *Common, stdout, stderr io.Writer) error {
	script, err := scriptPath(r.Script)
	if err != nil {
		return err
	}
	ctx := common.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return cli.RunContext(ctx, cli.RunOpts{
		Input:         common.input,
		Stdout:        stdout,
		HelperSources: helperSources,
		Stderr:        stderr,
		Verbose:       common.Verbose,
		Quiet:         common.Quiet,
		PrintMod:      r.PrintMod,
		PrintMain:     r.PrintMain,
		NoCache:       r.NoCache,
		ScriptPath:    script,
		Args:          r.Args,
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

func (b *buildArgs) Run(common *Common, stdout, stderr io.Writer) error {
	return cli.BuildContext(common.ctx, cli.BuildOpts{
		Input: common.input, HelperSources: helperSources, Stdout: stdout, Stderr: stderr,
		Verbose: common.Verbose, Quiet: common.Quiet, ScriptPath: b.Script, Output: b.Output, Force: b.Force, NoCache: b.NoCache,
	})
}

func cacheOptions(common *Common, stdout, stderr io.Writer, operation string, json bool) cli.CacheOpts {
	return cli.CacheOpts{Operation: operation, JSON: json, Verbose: common.Verbose, Quiet: common.Quiet, Stdout: stdout, Stderr: stderr}
}

func (c *cachePathArgs) Run(common *Common, stdout, stderr io.Writer) error {
	return cli.CacheContext(common.ctx, cacheOptions(common, stdout, stderr, "path", c.JSON))
}

func (c *cacheListArgs) Run(common *Common, stdout, stderr io.Writer) error {
	return cli.CacheContext(common.ctx, cacheOptions(common, stdout, stderr, "list", c.JSON))
}

func (c *cacheCleanArgs) Run(common *Common, stdout, stderr io.Writer) error {
	options := cacheOptions(common, stdout, stderr, "clean", c.JSON)
	options.KeyPrefix, options.All, options.Yes, options.DryRun = c.KeyPrefix, c.All, c.Yes, c.DryRun
	return cli.CacheContext(common.ctx, options)
}

func (c *cachePruneArgs) Run(common *Common, stdout, stderr io.Writer) error {
	options := cacheOptions(common, stdout, stderr, "prune", c.JSON)
	options.MaxAge, options.MaxBytes, options.Yes, options.DryRun = c.MaxAge, c.MaxBytes, c.Yes, c.DryRun
	return cli.CacheContext(common.ctx, options)
}
