package screen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WritePNG writes a picture to path, a file the caller named outside every
// directory rewake keeps state in (docs/screen.md). It refuses a path inside
// one of keep, a path that goes through a symbolic link, and an existing file
// that is not a regular one, so a request can never overwrite a record or
// follow a link planted in its way. The file is written whole and put in place
// by a rename. It returns the path as written.
func WritePNG(path string, data []byte, keep []string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(abs)
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("the folder of %s cannot be used: %w", path, err)
	}
	if resolved != dir {
		return "", fmt.Errorf("%s goes through a symbolic link (%s is %s): name the resolved path", path, dir, resolved)
	}
	for _, folder := range keep {
		if inside(resolved, folder) {
			return "", fmt.Errorf("%s is inside rewake's state (%s): write the picture elsewhere", path, folder)
		}
	}
	info, err := os.Lstat(abs)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return "", err
	case !info.Mode().IsRegular():
		return "", fmt.Errorf("%s exists and is not a regular file (a link, a folder or a device): name another path", path)
	}
	tmp, err := os.CreateTemp(dir, ".rewake-screen-*")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), abs); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return abs, nil
}

// inside is whether dir is folder or lies under it, folder's own links
// resolved where it exists.
func inside(dir, folder string) bool {
	if resolved, err := filepath.EvalSymlinks(folder); err == nil {
		folder = resolved
	}
	folder = filepath.Clean(folder)
	return dir == folder || strings.HasPrefix(dir, folder+string(filepath.Separator))
}
