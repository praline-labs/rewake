package host

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

// memoryFile is a file in memory alone, which an exec keeps once its
// close-on-exec is cleared: the face's model goes to the next image in one,
// never through a file on disk (docs/face.md), and it is past what the
// exec's environment takes.
func memoryFile(name string, data []byte) (*os.File, error) {
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
