package app

import "errors"

type BuildOpts struct {
	Verbose bool
}

func BuildCmd(o BuildOpts) error {
	return errors.New("build command not implemented")
}
