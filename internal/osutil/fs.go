package osutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// ReplaceDirContents empties a directory and writes the file system into it.
// It makes the directory when there is none.
func ReplaceDirContents(dir string, fsys fs.FS) error {
	if err := ClearDirContents(dir); err != nil {
		return err
	}
	return os.CopyFS(dir, fsys)
}

// ClearDirContents removes everything inside a directory, and keeps the
// directory itself. A directory that does not exist is not an error.
func ClearDirContents(dir string) error {
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Ignore the directory itself
		if path == dir {
			return nil
		}

		// But remove everything else inside it.
		if err := os.RemoveAll(path); err != nil {
			return err
		}

		if d.IsDir() {
			return filepath.SkipDir
		}

		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	return err
}
