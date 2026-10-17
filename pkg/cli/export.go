package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func outputPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("output path is required")
	}
	// Refuse a requested directory/link before adding a native extension.
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return "", fmt.Errorf("output must be a regular file, never a symlink or directory: %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if runtime.GOOS == "windows" && filepath.Ext(path) == "" {
		path += ".exe"
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, err := os.Stat(filepath.Dir(absolute))
	if err != nil {
		return "", fmt.Errorf("output parent: %w", err)
	}
	if !parent.IsDir() {
		return "", fmt.Errorf("output parent is not a directory")
	}

	return absolute, nil
}

func checkDestination(path, source string, force bool) error {
	if source != "" {
		absolute, err := filepath.Abs(source)
		if err != nil {
			return err
		}
		if path == absolute || (runtime.GOOS == "windows" && strings.EqualFold(path, absolute)) {
			return fmt.Errorf("output must not replace source: %s", path)
		}
	}
	info, err := os.Lstat(path) //nolint:gosec // G703: explicit operator-selected output, checked before publication.
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("output must be a regular file, never a symlink or directory: %s", path)
	}
	if source != "" {
		sourceInfo, err := os.Stat(source)
		if err == nil && os.SameFile(info, sourceInfo) {
			return fmt.Errorf("output must not replace source: %s", path)
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if !force {
		return fmt.Errorf("output already exists (use --force to replace a regular file): %s", path)
	}
	return nil
}

func exportExecutable(ctx context.Context, source, destination, scriptPath string, force bool) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkDestination(destination, scriptPath, force); err != nil {
		return err
	}
	input, err := os.Open(source) //nolint:gosec // G304: validated executable under a held cache/stage lease.
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, input.Close()) }()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("built executable is not regular")
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".gorn-export-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = temp.Close()
		if err := os.Remove(temp.Name()); err != nil && !os.IsNotExist(err) {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove export staging: %w", err))
		}
	}()
	if _, err := io.Copy(temp, sourceReader{ctx: ctx, reader: input}); err != nil {
		return err
	}
	if err := temp.Chmod(0o755); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkDestination(destination, scriptPath, force); err != nil {
		return err
	}
	// The publication operation is the commit point. Never remove/truncate the
	// destination first, and never fall back to a non-atomic copy on failure.
	return publishExecutable(temp.Name(), destination, force)
}
