//go:build (!linux && !darwin && !windows) || ((linux || darwin) && !amd64 && !arm64)

package process

import (
	"fmt"
	"os/exec"
	"runtime"
)

type platformProcess struct{}

func prepareProcess(*exec.Cmd, processOptions) (*platformProcess, error) {
	return nil, fmt.Errorf("process ownership unsupported on %s/%s", runtime.GOOS, runtime.GOARCH)
}
func (*platformProcess) started(*exec.Cmd) error               { return nil }
func (*platformProcess) observe(*exec.Cmd) (bool, bool, error) { return false, false, nil }
func (*platformProcess) terminate(*exec.Cmd) error             { return nil }
func (*platformProcess) restore() error                        { return nil }
func (*platformProcess) close() error                          { return nil }

func (*platformProcess) ownershipLost(error) bool { return true }
