package cli

import "errors"

type BuildOpts struct {
	Verbose bool
}

func Build(o BuildOpts) error {
	return errors.New("build command not implemented")
}
