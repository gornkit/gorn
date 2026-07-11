package cli

import "errors"

type CacheOpts struct {
	Verbose bool
}

func Cache(o CacheOpts) error {
	return errors.New("cache command not implemented")
}
