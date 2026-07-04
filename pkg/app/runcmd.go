package app

import (
	"bytes"
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
	fmt.Printf("Path:         %s\n", s.Path)
	fmt.Printf("GoVersion:    %q\n", s.GoVersion)
	fmt.Printf("Module:       %q\n", s.Module)
	fmt.Printf("Requires:     %+v\n", s.Requires)
	fmt.Printf("PackageStart: %s\n", intPtrString(s.PackageStart))
	fmt.Printf("MainStart:    %s\n", intPtrString(s.MainStart))
	fmt.Println("--- PackageLines ---")
	fmt.Print(string(bytes.Join(s.PackageLines, nil)))
	fmt.Println("--- MainLines ---")
	fmt.Print(string(bytes.Join(s.MainLines, nil)))
	fmt.Println("--- end ---")
}

func intPtrString(p *int) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *p)
}
