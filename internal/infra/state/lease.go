package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"syscall"
	"time"
)

// The room's build lease as this package keeps it
// (docs/rules/state.md#the-rooms-lease): build.lock, held with flock,
// and build, the id of the room's writers. infra/build decides what an id is
// and when a lease is taken; this package opens and locks the file, keeps the
// record, and keeps which rooms this process holds a lease for — the
// authorization a write of the room asks for. Nothing else grants it: no
// variable, no switch.

const (
	leaseLockName   = "build.lock"
	buildRecordName = "build"
)

// leaseLockAttempts bounds how often a lock replaced under its holder is
// opened again before the acquisition gives up.
const leaseLockAttempts = 8

// leasePoll is how often a held lease lock is tried again.
const leasePoll = 10 * time.Millisecond

// ErrLeaseBusy means the lease lock was not taken within the caller's bound:
// a build switch in progress, or a holder that does not let go.
var ErrLeaseBusy = errors.New("the room's build lease is busy")

// ErrNoBuildRecord means the room has no build record: no launch has
// established a build there.
var ErrNoBuildRecord = errors.New("the room has no build record")

// ErrNoLeaseLock means the room has no build.lock: no launch completed the
// room, and only a launch creates it.
var ErrNoLeaseLock = errors.New("the room has no build lease")

// LeaseLock is the open build.lock of one room and how it is held.
type LeaseLock struct {
	room string
	file *os.File
	// create says the lock may be created when missing: a launch's.
	create bool
	// exclusive is set while the lock is held exclusive.
	exclusive bool
	held      bool
}

// OpenLeaseLock opens the room's build.lock, which a launch made, never
// creating it: a room without one is a room no launch completed, which a
// writer refuses as it found it, ErrNoLeaseLock. The descriptor is
// close-on-exec, as every file Go opens: a child takes its own lease, never
// its parent's. Nothing is locked yet.
func OpenLeaseLock(room string) (*LeaseLock, error) {
	return openLease(room, false)
}

// CreateLeaseLock opens the room's build.lock as OpenLeaseLock does, creating
// it with mode 0600 when missing — a launch's, the one acquisition that makes
// a room. Opened again after a replacement, it is created again too.
func CreateLeaseLock(room string) (*LeaseLock, error) {
	return openLease(room, true)
}

func openLease(room string, create bool) (*LeaseLock, error) {
	if err := viewWrite(opLock, filepath.Join(room, leaseLockName)); err != nil {
		return nil, err
	}
	file, err := openLeaseFile(room, create)
	if err != nil {
		return nil, err
	}
	return &LeaseLock{room: filepath.Clean(room), file: file, create: create}, nil
}

func openLeaseFile(room string, create bool) (*os.File, error) {
	path := filepath.Join(room, leaseLockName)
	flags := os.O_RDWR | syscall.O_CLOEXEC | syscall.O_NOFOLLOW
	if create {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			// Creating it is a write a fault plan may end or fail; opening
			// the one there is not.
			if err := fault(OpWrite, path); err != nil {
				return nil, err
			}
		}
		flags |= os.O_CREATE
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if !create && errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNoLeaseLock, path)
	}
	if err != nil {
		return nil, fmt.Errorf("could not open the build lease of %s: %w", room, err)
	}
	return file, nil
}

// File is the lock's identity, for naming its holders: the device and inode
// of the file it holds.
func (l *LeaseLock) File() (dev, ino uint64, err error) {
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(l.file.Fd()), &stat); err != nil {
		return 0, 0, err
	}
	return stat.Dev, stat.Ino, nil
}

// Share takes the lock shared, waiting at most wait: an exclusive holder
// switching the build keeps it for a write and a sync, and one that stalls
// must not stall every writer behind it. Taken, the open file must still be
// the one at the path, or it is opened again.
func (l *LeaseLock) Share(wait time.Duration) error {
	return l.take(syscall.LOCK_SH, wait)
}

// TryExclusive takes the lock exclusive without waiting and says whether it
// did. It converts the lock this descriptor holds; flock does not promise the
// conversion keeps the shared lock if it fails, so a refusal leaves the lock,
// and its authorization, to be taken again or closed.
func (l *LeaseLock) TryExclusive() (bool, error) {
	err := l.take(syscall.LOCK_EX, 0)
	if errors.Is(err, ErrLeaseBusy) {
		return false, nil
	}
	return err == nil, err
}

// take locks the descriptor as how asks. A failure counts the lock lost,
// whatever it held before: a conversion may drop the old lock before it
// refuses, and a replaced file is closed with its lock.
func (l *LeaseLock) take(how int, wait time.Duration) (err error) {
	defer func() {
		if err != nil {
			l.lose()
		}
	}()
	deadline := time.Now().Add(wait)
	for attempt := 0; ; attempt++ {
		if err := l.lockBy(how, deadline); err != nil {
			return err
		}
		same, err := l.atPath()
		if err != nil {
			return err
		}
		if same {
			l.held, l.exclusive = true, how == syscall.LOCK_EX
			return nil
		}
		// Replaced or removed while this descriptor waited: its lock locks
		// nothing anyone else opens.
		if attempt+1 >= leaseLockAttempts {
			return fmt.Errorf("the build lease of %s was replaced %d times while it was taken", l.room, leaseLockAttempts)
		}
		_ = l.file.Close()
		file, err := openLeaseFile(l.room, l.create)
		if err != nil {
			return err
		}
		l.file = file
	}
}

func (l *LeaseLock) lockBy(how int, deadline time.Time) error {
	for {
		err := syscall.Flock(int(l.file.Fd()), how|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EINTR {
			return fmt.Errorf("could not lock the build lease of %s: %w", l.room, err)
		}
		if !time.Now().Before(deadline) {
			return ErrLeaseBusy
		}
		time.Sleep(leasePoll)
	}
}

// atPath says whether the open file is the file at the lock's path.
func (l *LeaseLock) atPath() (bool, error) {
	var open, named syscall.Stat_t
	if err := syscall.Fstat(int(l.file.Fd()), &open); err != nil {
		return false, err
	}
	err := syscall.Lstat(filepath.Join(l.room, leaseLockName), &named)
	if errors.Is(err, syscall.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return open.Dev == named.Dev && open.Ino == named.Ino, nil
}

// Unlock lets go of whatever this descriptor holds, keeping it open to take
// the lock again: a launch that lost the race to switch the build waits
// unlocked, so the winner can take the lock exclusive.
func (l *LeaseLock) Unlock() error {
	// Whether or not the unlock answers, the lock is no longer one to write by.
	l.lose()
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("could not unlock the build lease of %s: %w", l.room, err)
	}
	return nil
}

// lose marks the lock not held and voids its authorization: no write is let
// through by a lock that may hold nothing.
func (l *LeaseLock) lose() {
	l.held, l.exclusive = false, false
	revoke(l)
}

// Close releases the lock and its authorization. The file stays: removing it
// would let the next writer lock a new inode while an old holder keeps the
// old one.
func (l *LeaseLock) Close() {
	Note("lease release")
	l.lose()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// buildShape is a build record: a SHA-256 in lower-case hex.
var buildShape = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ReadBuildRecord reads the room's build. A missing record is
// ErrNoBuildRecord; one that cannot be read, or does not hold an id, is an
// error that says so — never an id.
func ReadBuildRecord(room string) (string, error) {
	data, err := os.ReadFile(filepath.Join(room, buildRecordName))
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNoBuildRecord
	}
	if err != nil {
		return "", fmt.Errorf("could not read the build record of %s: %w", room, err)
	}
	id := string(data)
	if len(id) > 0 && id[len(id)-1] == '\n' {
		id = id[:len(id)-1]
	}
	if !buildShape.MatchString(id) {
		return "", fmt.Errorf("the build record of %s holds no build id", room)
	}
	return id, nil
}

// WriteBuildRecord writes the room's build, by rename and sync, under the
// exclusive lock this lease lock holds: nobody else holds the lease, so no
// writer depends on the record it replaces.
func WriteBuildRecord(l *LeaseLock, id string) error {
	if !l.held || !l.exclusive {
		return fmt.Errorf("the build record of %s is written only under the exclusive lease", l.room)
	}
	if !buildShape.MatchString(id) {
		return fmt.Errorf("%q is no build id", id)
	}
	path := filepath.Join(l.room, buildRecordName)
	if err := viewWrite(OpWrite, path); err != nil {
		return err
	}
	return replace(path, []byte(id+"\n"), nil)
}

// The rooms this process holds a lease for. A write of a room asks here
// (docs/rules/state.md#what-is-a-writer); the entry goes when the lock
// is closed.
var leases struct {
	mu   sync.Mutex
	held map[string][]*LeaseLock
}

// Authorize records that this process holds the room's lease through this
// lock, shared, with its build checked. infra/build calls it once the id
// matched. A lock is recorded once, however often it is authorized, so the
// one revocation its release makes leaves nothing of it behind.
func Authorize(l *LeaseLock) error {
	if !l.held {
		return fmt.Errorf("the build lease of %s is not held", l.room)
	}
	leases.mu.Lock()
	defer leases.mu.Unlock()
	if leases.held == nil {
		leases.held = map[string][]*LeaseLock{}
	}
	if !slices.Contains(leases.held[l.room], l) {
		leases.held[l.room] = append(leases.held[l.room], l)
	}
	return nil
}

func revoke(l *LeaseLock) {
	leases.mu.Lock()
	defer leases.mu.Unlock()
	held := leases.held[l.room]
	for index, lock := range held {
		if lock == l {
			leases.held[l.room] = append(held[:index:index], held[index+1:]...)
			break
		}
	}
	if len(leases.held[l.room]) == 0 {
		delete(leases.held, l.room)
	}
}

// Leased says whether this process holds the lease of the room.
func Leased(room string) bool {
	leases.mu.Lock()
	defer leases.mu.Unlock()
	return len(leases.held[filepath.Clean(room)]) > 0
}
