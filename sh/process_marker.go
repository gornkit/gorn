//go:build (linux || darwin) && (amd64 || arm64)

package sh

import (
	"os"
	"strconv"
	"syscall"
)

func init() {
	// A runner's child becomes its group leader before exec. Normalize only
	// this reserved coordination variable, before user init/main can spawn
	// commands. Numeric markers are always checked again against Getpgrp.
	if os.Getenv(ownershipVariable) == "self" && os.Getpid() == syscall.Getpgrp() {
		_ = os.Setenv(ownershipVariable, strconv.Itoa(syscall.Getpgrp()))
	}
}
