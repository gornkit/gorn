package app

import "errors"

type CacheOpts struct {
	Verbose bool
}

func CacheCmd(o CacheOpts) error {
	return errors.New("cache command not implemented")
}
