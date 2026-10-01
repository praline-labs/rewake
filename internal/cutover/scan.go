// Package cutover proves, before a launch of this build takes a name, that
// no rewake process of the earlier build can still write that name's mailbox
// (docs/protocol-cutover.md#proving-the-earlier-writers-stopped).
package cutover

import (
	"bufio"
	"bytes"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
)

// rewakeMain is the main package of the rewake binary, as its build
// information names it: every release so far was built from it.
const rewakeMain = "github.com/praline-labs/rewake/cmd/rewake"

// Kind is why a process blocks a launch.
type Kind int

const (
	// UnknownUser says the process's credentials cannot be read, so it is not
	// proven to be another user's.
	UnknownUser Kind = iota + 1
	// UnknownExecutable says a process of this user cannot have its
	// executable read, so it is not proven to be another program.
	UnknownExecutable
	// EarlierRewake says the process is a rewake of the earlier build.
	EarlierRewake
)

// Address is a session's whole address: its state root, room and name.
type Address struct{ Root, Room, Name string }

// Process is a process the look could not clear by itself: what it is, and
// for an earlier-build rewake the session it belongs to, when that is known.
type Process struct {
	PID    int
	Kind   Kind
	Detail string
	// Path is the executable's path, for an earlier-build rewake whose file
	// is still on disk: a path the upgrade did not replace.
	Path string
	// Owner is the session an earlier-build rewake belongs to; nil when it
	// cannot be attributed.
	Owner *Address
}

// Scanner lists a /proc-shaped tree once. Its fields describe the scanning
// process, so tests can describe another.
type Scanner struct {
	Proc proc.Reader
	UID  int
	// Caps is the scanning process's permitted and inheritable capabilities.
	Caps capabilities
	// Owner answers the user owning a file under the tree: the kernel gives
	// a process's files to root while it is not dumpable.
	Owner func(path string) (int, error)
	// Stamp is the bytes a binary of this build carries.
	Stamp []byte
	// BuildInfo reads a Go binary's build information.
	BuildInfo func(io.ReaderAt) (*buildinfo.BuildInfo, error)
}

// builtProcRoot is the process tree the look lists when a build sets one with
// -ldflags -X: the workflow suite points it at a tree of its own, so none of
// its launches depends on the processes of the machine it runs on. A release
// build sets none, and the look lists /proc.
var builtProcRoot string

// Tree is the process tree Here lists. Replaceable, so the tests of a package
// that launches do not depend on this machine's processes either.
var Tree = func() proc.Reader {
	if builtProcRoot != "" {
		return proc.Reader{Root: builtProcRoot}
	}
	return proc.Default
}()

// Here is the scanner of this process over Tree. Its own capabilities are
// always this process's, from the real /proc.
func Here() (Scanner, error) {
	s := Scanner{Proc: Tree, UID: os.Getuid(), Stamp: []byte(registry.BuildStamp), BuildInfo: buildinfo.Read, Owner: fileOwner}
	status, err := readStatus(filepath.Join(proc.Default.Root, "self", "status"))
	if err == nil && (!status.caps.hasPermitted || !status.caps.hasInheritable) {
		err = errors.New("no CapPrm or CapInh line")
	}
	if err != nil {
		return s, fmt.Errorf("this process's own capabilities cannot be read from /proc/self/status: %v", err)
	}
	s.Caps = status.caps
	return s, nil
}

// Look lists every process of the tree and answers those it could not clear:
// one of another user, one proven to be another program or this build, and
// one proven ended are not writers.
func (s Scanner) Look() ([]Process, error) {
	entries, err := os.ReadDir(s.Proc.Root)
	if err != nil {
		return nil, fmt.Errorf("%s cannot be listed, so no earlier-build writer is proven stopped: %w", s.Proc.Root, err)
	}
	var found []Process
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		if process, ok := s.inspect(pid); ok {
			found = append(found, process)
		}
	}
	return found, nil
}

// inspect answers what pid is, and false when it is cleared. The start is
// read before and after: a process that ended or was replaced meanwhile is
// not the one whose files were read, and has ended.
func (s Scanner) inspect(pid int) (Process, bool) {
	start, err := s.Proc.StartTime(pid)
	if gone(err) || err == nil && s.Proc.ObserveIdentity(pid, start) == proc.IdentityEnded {
		return Process{}, false
	}
	// A stat that cannot be read leaves the start unknown; the status then
	// says what the process is, and only its disappearance ends it.
	known := err == nil
	process, blocks := s.judge(pid, start)
	if !blocks {
		return Process{}, false
	}
	now, err := s.Proc.StartTime(pid)
	if gone(err) || known && err == nil && (now != start || s.Proc.ObserveIdentity(pid, now) == proc.IdentityEnded) {
		return Process{}, false
	}
	return process, true
}

// gone says a read of a process's files failed because it has ended.
func gone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH)
}

func (s Scanner) judge(pid int, start uint64) (Process, bool) {
	dir := filepath.Join(s.Proc.Root, strconv.Itoa(pid))
	// The Uid line, never the owner of the files: those belong to root when
	// the process is not dumpable, whoever it runs as.
	status, err := readStatus(filepath.Join(dir, "status"))
	if err != nil || !status.hasUID {
		return Process{PID: pid, Kind: UnknownUser, Detail: fmt.Sprintf("its user cannot be read from %s/status (%v)", dir, errOr(err, "no Uid line"))}, true
	}
	if !status.user(s.UID) {
		// Another user cannot write a state directory only its owner may.
		return Process{}, false
	}
	exe, err := os.Open(filepath.Join(dir, "exe"))
	if err != nil {
		if errors.Is(err, fs.ErrPermission) && s.cannotBeRewake(dir, status) {
			return Process{}, false
		}
		return Process{PID: pid, Kind: UnknownExecutable, Detail: fmt.Sprintf("it runs as this user and its executable cannot be read (%v), with no capability beyond this process's and dumpable, so it is not proven to be another program", err)}, true
	}
	defer func() { _ = exe.Close() }()
	info, err := s.BuildInfo(exe)
	if err != nil {
		var pathErr *fs.PathError
		var errno syscall.Errno
		if errors.As(err, &pathErr) || errors.As(err, &errno) {
			return Process{PID: pid, Kind: UnknownExecutable, Detail: fmt.Sprintf("its executable cannot be read to the end (%v)", err)}, true
		}
		// Not a Go binary: another program, whatever it carries.
		return Process{}, false
	}
	if info.Path != rewakeMain {
		return Process{}, false
	}
	stamped, err := contains(exe, s.Stamp)
	if err != nil {
		return Process{PID: pid, Kind: UnknownExecutable, Detail: fmt.Sprintf("it is a rewake whose executable cannot be read to the end (%v), so its build is unknown", err)}, true
	}
	if stamped {
		return Process{}, false
	}
	process := Process{PID: pid, Kind: EarlierRewake, Path: leftOnDisk(dir)}
	process.Owner, process.Detail = s.attribute(dir, pid, start)
	return process, true
}

// cannotBeRewake says why the kernel hides a process's executable, when the
// reason is one an earlier-build rewake inside the cutover's boundary cannot
// have: capabilities beyond this process's, which a rewake started by the
// person's harness or shell does not hold, or not being dumpable, which no
// earlier build makes itself (docs/protocol-cutover.md). The files of a
// process that is not dumpable belong to root; the Uid line already said it
// runs as this user.
func (s Scanner) cannotBeRewake(dir string, status status) bool {
	if status.caps.beyond(s.Caps) {
		return true
	}
	owner, err := s.Owner(filepath.Join(dir, "status"))
	return err == nil && owner == 0 && s.UID != 0
}

func fileOwner(path string) (int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("the owner of %s cannot be read", path)
	}
	return int(stat.Uid), nil
}

func errOr(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}

// leftOnDisk is the executable's path while that file is still there: the
// kernel marks one replaced or removed since the process started.
func leftOnDisk(dir string) string {
	path, err := os.Readlink(filepath.Join(dir, "exe"))
	if err != nil || strings.HasSuffix(path, " (deleted)") {
		return ""
	}
	return path
}

// contains streams r looking for stamp, which may straddle two reads.
func contains(r io.ReaderAt, stamp []byte) (bool, error) {
	reader := bufio.NewReaderSize(io.NewSectionReader(r, 0, 1<<62), 1<<20)
	window := make([]byte, 0, 1<<20+len(stamp))
	chunk := make([]byte, 1<<20)
	for {
		n, err := reader.Read(chunk)
		window = append(window, chunk[:n]...)
		if bytes.Contains(window, stamp) {
			return true, nil
		}
		if keep := len(stamp) - 1; len(window) > keep {
			window = append(window[:0], window[len(window)-keep:]...)
		}
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}
