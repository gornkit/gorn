package app

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gornkit/gorn/pkg/gornparser"
)

type RunOpts struct {
	Verbose  bool
	PrintGen bool
	Script   string
	Args     []string
}

func RunCmd(o RunOpts) error {
	fmt.Printf("Running script: %s\n", o.Script)
	fmt.Printf("Arguments: %v\n", o.Args)
	if o.Verbose {
		fmt.Println("Verbose mode enabled")
	}

	script, err := parseScript(o.Script)
	if err != nil {
		return fmt.Errorf("parse %s: %w", o.Script, err)
	}

	dumpScript(script)

	if o.PrintGen {
		gen, err := gornparser.Generate(script)
		if err != nil {
			// A format failure carries the raw, unformatted main file; dump
			// it for debugging before surfacing the error.
			var genErr *gornparser.GenerateError
			if errors.As(err, &genErr) && genErr.Raw != nil {
				fmt.Println("--- generated main file (unformatted) ---")
				fmt.Print(string(genErr.Raw))
			}
			return fmt.Errorf("generate: %w", err)
		}

		fmt.Println("--- generated mod file ---")
		fmt.Print(string(gen.ModGenerated))
		fmt.Println("--- generated main file (unformatted) ---")
		fmt.Print(string(gen.MainGenerated))
		fmt.Println("--- generated main file ---")
		fmt.Print(string(gen.MainFileFormatted))
	}

	return nil
}

// parseScript is a thin wrapper over gornparser, handling the "-" (stdin)
// convention that scriptPath already validated for us.
func parseScript(path string) (*gornparser.Script, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		return gornparser.ParseSource("-", data)
	}
	return gornparser.ParseFile(path)
}

// dumpScript is a quick-and-dirty debug dump of a parsed Script, useful
// until a real code generator exists to actually do something with it.
func dumpScript(s *gornparser.Script) {
	fmt.Println("--- parsed script ---")
	fmt.Printf("Path:         %s\n", s.SourcePath)
	fmt.Printf("GoVersion:    %q\n", s.GoVersion)
	fmt.Printf("Module:       %q\n", s.Module)
	fmt.Printf("Requires:     %+v\n", s.Requires)
	fmt.Printf("PackageStart: %s\n", intPtrString(s.PackageStart))
	fmt.Printf("MainStart:    %d\n", s.MainStart)
	fmt.Println("--- PackageLines ---")
	fmt.Println(s.PackageContent)
	fmt.Println("--- MainLines ---")
	fmt.Println(s.MainContent)
	fmt.Println("--- end ---")
}

func intPtrString(p *int) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *p)
}
