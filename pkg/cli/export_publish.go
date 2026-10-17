package cli

import (
	"errors"
	"os"
	"path/filepath"
)

func publishExecutable(staged, destination string, force bool) (resultErr error) {
	root, err := os.OpenRoot(filepath.Dir(destination))
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, root.Close()) }()
	oldName, newName := filepath.Base(staged), filepath.Base(destination)
	if force {
		// With the pinned Go runtime, Root.Rename uses renameat on Unix and a
		// native rename attempts on Windows (extended, then legacy if needed).
		// No copy/delete or multi-step ReplaceFile fallback discards old output.
		return root.Rename(oldName, newName)
	}
	// Atomic no-replace creation. A hard-link failure is explicit; no racy
	// check-then-rename or partially visible copy fallback is permitted.
	return root.Link(oldName, newName)
}
