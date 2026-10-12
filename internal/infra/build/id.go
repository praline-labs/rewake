/*
Package build holds one build per room (docs/v2/design-state.md#one-build-per-room):
the build id, which is the SHA-256 of the running executable's content, and the
room's lease, which every writer of the room holds shared and a launch that
changes the room's build takes exclusive.

The version line is no identity — two builds of one dirty tree print the same
one — and neither is the file at the install path, which an upgrade replaces
under running sessions. The image the process runs is: /proc/self/exe opens it
even after the file at its path is gone.

This package decides what an id is and when the lease is taken; infra/state
opens and locks the files and keeps the authorization, and never imports this
package back.
*/
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
)

// image is the executable whose content is the id; a test points it at a
// file of its own.
var image = "/proc/self/exe"

// ID returns this process's build id, hashing its image once, on first use,
// over a streamed read. An image that cannot be opened or read gives no id:
// whoever needs one refuses before its first effect.
func ID() (string, error) { return id() }

var id = sync.OnceValues(func() (string, error) { return hashFile(image) })

func hashFile(path string) (string, error) {
	id, err := FileID(path)
	if err != nil {
		return "", fmt.Errorf("rewake cannot read its own executable to know its build (%w), so it changes nothing", err)
	}
	return id, nil
}

func hashReader(r io.Reader) (string, error) {
	sum := sha256.New()
	if _, err := io.Copy(sum, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// FileID is the id of another executable, read the same way: the peer of a
// connection is checked by the content it runs, as a room's writers are.
func FileID(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	return hashReader(file)
}

// Short is an id as a person reads it: its first twelve digits.
func Short(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
