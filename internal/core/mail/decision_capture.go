package mail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
)

// A decision keeps the observations its final plan used (Q12 of
// docs/v2/stage3-decision-build.md): each read as the plan consumed it, in
// order, through the same seam the plan reads by — never a second snapshot
// taken beside it, which could see another state of the mailbox, and never
// the plan's audit strings. capture is that seam: it hands every read on to
// the file system and keeps a copy of what came back, the bytes of a read
// that will fail to parse included, and for a read that failed its error and
// what lstat said of the path. What could not be read is not in it.

// The operations an observation records.
const (
	observedRead  = "read"
	observedList  = "list"
	observedStat  = "stat"
	observedLstat = "lstat"
)

// Observation is one read of a decision's final plan.
type Observation struct {
	Op string `json:"op"`
	// Path is under the state directory, as a stop record names one.
	Path    string          `json:"path"`
	Bytes   []byte          `json:"bytes,omitempty"`
	SHA256  string          `json:"sha256,omitempty"`
	// Entries is set for a listing alone, empty when it listed nothing:
	// omitzero keeps an empty listing apart from none.
	Entries []ObservedEntry `json:"entries,omitzero"`
	Info    *ObservedInfo   `json:"info,omitempty"`
	Err     *ObservedError  `json:"err,omitempty"`
	// Lstat is what lstat answered of the path when the read failed.
	Lstat *ObservedStat `json:"lstat,omitempty"`
}

// ObservedEntry is one entry of a listing.
type ObservedEntry struct {
	Name string `json:"name"`
	Mode uint32 `json:"mode"`
}

// ObservedInfo is what a stat said.
type ObservedInfo struct {
	Mode uint32 `json:"mode"`
	Size int64  `json:"size"`
}

// ObservedError is a read that failed, by kind.
type ObservedError struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// ObservedStat is lstat's own outcome.
type ObservedStat struct {
	Info *ObservedInfo  `json:"info,omitempty"`
	Err  *ObservedError `json:"err,omitempty"`
}

// The kinds of a failed read.
const (
	errNotExist   = "not-exist"
	errNotDir     = "not-directory"
	errPermission = "permission"
	errOther      = "other"
	maxErrText    = 4096
)

func observedError(err error) *ObservedError {
	kind := errOther
	switch {
	case errors.Is(err, os.ErrNotExist):
		kind = errNotExist
	case errors.Is(err, syscall.ENOTDIR):
		kind = errNotDir
	case errors.Is(err, os.ErrPermission):
		kind = errPermission
	}
	text := err.Error()
	if len(text) > maxErrText {
		text = text[:maxErrText]
	}
	return &ObservedError{Kind: kind, Text: text}
}

func observedInfo(info fs.FileInfo) *ObservedInfo {
	return &ObservedInfo{Mode: uint32(info.Mode()), Size: info.Size()}
}

// ErrObservationBound refuses a plan whose reads pass what a decision may
// keep: nothing accepted is truncated, so such a plan makes no decision.
var ErrObservationBound = errors.New("the reads of the plan pass what a decision may keep")

// capture is the seam a final plan reads through. It writes nothing.
type capture struct {
	files fileAccess
	dir   string
	mu    sync.Mutex
	log   []Observation
	total int
	over  error
}

func newCapture(files fileAccess, dir string) *capture { return &capture{files: files, dir: dir} }

func (c *capture) keep(o Observation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total += len(o.Bytes)
	switch {
	case len(o.Bytes) > maxObservedRead:
		c.over = fmt.Errorf("%w: %s is %d bytes", ErrObservationBound, o.Path, len(o.Bytes))
	case c.total > maxObservedTotal:
		c.over = fmt.Errorf("%w: %d bytes in all", ErrObservationBound, c.total)
	case len(o.Entries) > maxObservedEntries:
		c.over = fmt.Errorf("%w: %s lists %d entries", ErrObservationBound, o.Path, len(o.Entries))
	case len(c.log) >= maxObservedEntries:
		c.over = fmt.Errorf("%w: more than %d reads", ErrObservationBound, maxObservedEntries)
	}
	if c.over == nil {
		c.log = append(c.log, o)
	}
}

// Observations are the reads so far, in order, each a copy no later read
// changes; or the bound they passed.
func (c *capture) Observations() ([]Observation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.over != nil {
		return nil, c.over
	}
	out := make([]Observation, len(c.log))
	for i, o := range c.log {
		o.Bytes = slices.Clone(o.Bytes)
		o.Entries = slices.Clone(o.Entries)
		o.Info, o.Err = clonePtr(o.Info), clonePtr(o.Err)
		if o.Lstat != nil {
			o.Lstat = &ObservedStat{Info: clonePtr(o.Lstat.Info), Err: clonePtr(o.Lstat.Err)}
		}
		out[i] = o
	}
	return out, nil
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func (c *capture) failed(op, path string, err error) Observation {
	o := Observation{Op: op, Path: relative(c.dir, path), Err: observedError(err)}
	if op != observedLstat {
		stat := &ObservedStat{}
		if info, lerr := c.files.Lstat(path); lerr != nil {
			stat.Err = observedError(lerr)
		} else {
			stat.Info = observedInfo(info)
		}
		o.Lstat = stat
	}
	return o
}

func (c *capture) ReadFile(path string) ([]byte, error) {
	raw, err := c.files.ReadFile(path)
	if err != nil {
		c.keep(c.failed(observedRead, path, err))
		return raw, err
	}
	sum := sha256.Sum256(raw)
	c.keep(Observation{Op: observedRead, Path: relative(c.dir, path), Bytes: slices.Clone(raw), SHA256: hex.EncodeToString(sum[:])})
	return raw, nil
}

func (c *capture) ReadDir(path string) ([]fs.DirEntry, error) {
	entries, err := c.files.ReadDir(path)
	if err != nil {
		c.keep(c.failed(observedList, path, err))
		return entries, err
	}
	o := Observation{Op: observedList, Path: relative(c.dir, path), Entries: []ObservedEntry{}}
	for _, entry := range entries {
		o.Entries = append(o.Entries, ObservedEntry{Name: entry.Name(), Mode: uint32(entry.Type())})
	}
	c.keep(o)
	return entries, nil
}

func (c *capture) stat(op string, path string, read func(string) (fs.FileInfo, error)) (fs.FileInfo, error) {
	info, err := read(path)
	if err != nil {
		c.keep(c.failed(op, path, err))
		return info, err
	}
	c.keep(Observation{Op: op, Path: relative(c.dir, path), Info: observedInfo(info)})
	return info, nil
}

func (c *capture) Stat(path string) (fs.FileInfo, error) {
	return c.stat(observedStat, path, c.files.Stat)
}

func (c *capture) Lstat(path string) (fs.FileInfo, error) {
	return c.stat(observedLstat, path, c.files.Lstat)
}

var errCaptureWrites = errors.New("a plan's capture writes nothing")

func (c *capture) WriteFile(string, []byte) error { return errCaptureWrites }
func (c *capture) Publish(string, []byte) error   { return errCaptureWrites }
func (c *capture) EnsureDir(string) error         { return errCaptureWrites }
func (c *capture) Remove(string) error            { return errCaptureWrites }
func (c *capture) Rename(string, string) error    { return errCaptureWrites }
func (c *capture) SyncDir(string) error           { return errCaptureWrites }

// Lock is the caller's: a final plan reads inside the locks its caller took,
// and a capture takes none of its own.
func (c *capture) Lock(context.Context, string, func() error) error { return errCaptureWrites }

// checkObservations holds a decision's observations to their shape and
// bounds, and each kept byte string to its hash.
func checkObservations(observations []Observation) error {
	if len(observations) == 0 {
		return errors.New("it keeps no observation")
	}
	if len(observations) > maxObservedEntries {
		return fmt.Errorf("it keeps %d observations", len(observations))
	}
	total := 0
	for i, o := range observations {
		total += len(o.Bytes)
		if err := o.check(); err != nil {
			return fmt.Errorf("its observation %d, %s of %s: %w", i, o.Op, o.Path, err)
		}
	}
	if total > maxObservedTotal {
		return fmt.Errorf("its observations keep %d bytes, past %d", total, maxObservedTotal)
	}
	return nil
}

func (o Observation) check() error {
	if o.Path == "" || filepath.IsAbs(o.Path) || filepath.Clean(o.Path) != o.Path || o.Path == ".." ||
		strings.HasPrefix(o.Path, ".."+string(filepath.Separator)) {
		return errors.New("it names no path under the state directory")
	}
	if !slices.Contains([]string{observedRead, observedList, observedStat, observedLstat}, o.Op) {
		return errors.New("it is no read a plan makes")
	}
	if o.Err != nil {
		return o.checkFailed()
	}
	if o.Lstat != nil {
		return errors.New("it keeps an lstat beside a read that returned")
	}
	switch o.Op {
	case observedRead:
		sum := sha256.Sum256(o.Bytes)
		if o.SHA256 != hex.EncodeToString(sum[:]) || o.Entries != nil || o.Info != nil || len(o.Bytes) > maxObservedRead {
			return errors.New("its bytes do not match their hash, or past their bound")
		}
	case observedList:
		if o.Entries == nil || len(o.Entries) > maxObservedEntries || o.Bytes != nil || o.SHA256 != "" || o.Info != nil {
			return errors.New("it is a listing that lists nothing, or past its bound")
		}
	default:
		if o.Info == nil || o.Bytes != nil || o.SHA256 != "" || o.Entries != nil {
			return errors.New("it is a stat that says nothing")
		}
	}
	return nil
}

func (o Observation) checkFailed() error {
	if o.Bytes != nil || o.SHA256 != "" || o.Entries != nil || o.Info != nil || !o.Err.valid() {
		return errors.New("a failed read keeps what was not read, or no error")
	}
	if (o.Op == observedLstat) != (o.Lstat == nil) {
		return errors.New("a failed read keeps no lstat, or an lstat keeps one")
	}
	if o.Lstat != nil && (o.Lstat.Info == nil) == (o.Lstat.Err == nil || !o.Lstat.Err.valid()) {
		return errors.New("its lstat says neither what it found nor why it failed")
	}
	return nil
}

func (e *ObservedError) valid() bool {
	return e != nil && slices.Contains([]string{errNotExist, errNotDir, errPermission, errOther}, e.Kind) &&
		e.Text != "" && len(e.Text) <= maxErrText
}
