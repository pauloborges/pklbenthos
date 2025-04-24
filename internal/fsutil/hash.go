// Package fsutil holds helpers that work on a file system.
package fsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
)

// Hash returns a digest of the contents of a file system.
//
// Two file systems give the same digest when they hold the same files with the
// same bytes. The digest covers the name and the size of each file as well, so
// that a move of bytes from one file to another gives a digest of its own.
//
// A generated library carries no date and no version of its own, so this
// digest tells whether one release of a build changed the library that comes
// out of it.
func Hash(fsys fs.FS) (string, error) {
	digest := sha256.New()

	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}

		// WalkDir reads a directory in the order of the alphabet, so the
		// digest does not depend on the order of the file system itself.
		fmt.Fprintf(digest, "%s\x00%d\x00", name, len(data))
		digest.Write(data)

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("hash the file system: %w", err)
	}

	return hex.EncodeToString(digest.Sum(nil)), nil
}
