package app

import (
	"fmt"
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
	return nil
}
