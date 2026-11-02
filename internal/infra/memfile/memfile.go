// Package memfile makes files in memory alone, for what an image hands to
// the next across an exec: a wrapper's face model, a space host's screen.
package memfile

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

// New is a file in memory alone holding data, read from its start, which an
// exec keeps once its close-on-exec is cleared: what goes to the next image
// in one never lies in a file on disk (docs/face.md), and it is past what
// the exec's environment takes.
func New(name string, data []byte) (*os.File, error) {
	path, err := syscall.BytePtrFromString(name)
	if err != nil {
		return nil, err
	}
	fd, _, errno := syscall.Syscall(sysMemfdCreate, uintptr(unsafe.Pointer(path)), memfdCloexec, 0)
	if errno != 0 {
		return nil, errno
	}
	file := os.NewFile(fd, name)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// memfdCloexec is MFD_CLOEXEC: the file goes to no child, and to the next
// image only when the exec clears it.
const memfdCloexec = 1
