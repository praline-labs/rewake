package inbox

import (
	"context"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// seamRead is one read through the seam: its operation and its path under
// the state directory, the same in every copy of a scene.
type seamRead struct{ op, rel string }

// seamFault fails one read: the path cannot be followed, is closed, is a
// directory where a file belongs, or holds what its kind does not read as.
type seamFault struct {
	at      seamRead
	kind    string
	garbage []byte
}

// seamProbe stands for the file system in one pass: it records every read
// and fails the one its fault names. Writes go through.
type seamProbe struct {
	base  fileAccess
	dir   string
	fault *seamFault

	mu    sync.Mutex
	reads map[seamRead]bool
	hits  int

	// writes are the writes in their order; broken is the one that fails,
	// and broke whether it did.
	writes []seamWrite
	broken *seamWrite
	broke  bool
	// occurrences are the ids of the occurrences this pass recorded: a write
	// that names one — main's note of it — is the same write in another run.
	occurrences []string
}

// seamWrite is one write through the seam: the n-th of its operation on its
// path, so the same write is found again in another run of a scene.
type seamWrite struct {
	op, rel string
	n       int
}

func (w seamWrite) String() string { return fmt.Sprintf("%s %s (#%d)", w.op, w.rel, w.n) }

// occurrenceName is a stop occurrence's record or one beside it, by the
// occurrence's id.
var occurrenceName = regexp.MustCompile(`^(inbox/[^/]+/` + stopsDir + `/[0-9a-f]+/)[0-9]{19}-[0-9a-f]{12}(.*)$`)

// sameOccurrence names a write of a stop's records without the occurrence's
// id, which is fresh in every run: the same write is found again in another
// run of a scene by its key, which is not.
func sameOccurrence(rel string) string {
	return occurrenceName.ReplaceAllString(filepath.ToSlash(rel), "${1}<occurrence>${2}")
}

func newProbe(dir string, base fileAccess, fault *seamFault) *seamProbe {
	return &seamProbe{base: base, dir: dir, fault: fault, reads: map[seamRead]bool{}}
}

// wrote records a write and answers the failure it meets, if it is the
// broken one: a write that fails before it changes anything, as
// state.WriteAtomic does when its temporary file cannot be made.
func (p *seamProbe) wrote(op, path string) error {
	rel, err := filepath.Rel(p.dir, path)
	if err != nil {
		rel = path
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if match := occurrenceName.FindStringSubmatch(filepath.ToSlash(rel)); match != nil && match[2] == "" {
		p.occurrences = append(p.occurrences, strings.TrimPrefix(filepath.ToSlash(rel), match[1]))
	}
	rel = sameOccurrence(rel)
	for _, id := range p.occurrences {
		rel = strings.ReplaceAll(rel, id, "<occurrence>")
	}
	at := seamWrite{op: op, rel: rel, n: 1}
	for _, earlier := range p.writes {
		if earlier.op == op && earlier.rel == rel {
			at.n++
		}
	}
	p.writes = append(p.writes, at)
	if p.broken != nil && *p.broken == at {
		p.broke = true
		return &fs.PathError{Op: op, Path: path, Err: syscall.EIO}
	}
	return nil
}

// met records a read and answers the fault it meets, if any.
func (p *seamProbe) met(op, path string) *seamFault {
	rel, err := filepath.Rel(p.dir, path)
	if err != nil {
		rel = path
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	at := seamRead{op, rel}
	p.reads[at] = true
	if p.fault != nil && p.fault.at == at {
		p.hits++
		return p.fault
	}
	return nil
}

// arm puts fault in place from now on, counting only the hits it meets.
func (p *seamProbe) arm(fault *seamFault) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fault, p.hits = fault, 0
}

func (p *seamProbe) failure(op, path string, fault *seamFault) error {
	errno := map[string]syscall.Errno{"not a directory": syscall.ENOTDIR, "no access": syscall.EACCES, "a directory": syscall.EISDIR}[fault.kind]
	return &fs.PathError{Op: op, Path: path, Err: errno}
}

func (p *seamProbe) ReadFile(path string) ([]byte, error) {
	if fault := p.met("read", path); fault != nil {
		if fault.kind == "unparseable" {
			return fault.garbage, nil
		}
		return nil, p.failure("read", path, fault)
	}
	return p.base.ReadFile(path)
}

func (p *seamProbe) ReadDir(path string) ([]fs.DirEntry, error) {
	if fault := p.met("list", path); fault != nil {
		return nil, p.failure("open", path, fault)
	}
	return p.base.ReadDir(path)
}

func (p *seamProbe) Stat(path string) (fs.FileInfo, error) {
	if fault := p.met("stat", path); fault != nil {
		if fault.kind == "a directory" {
			return directoryInfo(filepath.Base(path)), nil
		}
		return nil, p.failure("stat", path, fault)
	}
	return p.base.Stat(path)
}

func (p *seamProbe) WriteFile(path string, raw []byte) error {
	if err := p.wrote("write", path); err != nil {
		return err
	}
	return p.base.WriteFile(path, raw)
}

func (p *seamProbe) Publish(path string, raw []byte) error {
	if err := p.wrote("publish", path); err != nil {
		return err
	}
	return p.base.Publish(path, raw)
}

func (p *seamProbe) EnsureDir(path string) error {
	if err := p.wrote("make", path); err != nil {
		return err
	}
	return p.base.EnsureDir(path)
}

func (p *seamProbe) Remove(path string) error {
	if err := p.wrote("remove", path); err != nil {
		return err
	}
	return p.base.Remove(path)
}

func (p *seamProbe) Rename(from, to string) error {
	if err := p.wrote("rename", from); err != nil {
		return err
	}
	return p.base.Rename(from, to)
}

// SyncDir fails after the change it would make durable, as a sync does.
func (p *seamProbe) SyncDir(path string) error {
	if err := p.base.SyncDir(path); err != nil {
		return err
	}
	return p.wrote("sync", path)
}

func (p *seamProbe) Lock(ctx context.Context, mailbox string, fn func() error) error {
	return p.base.Lock(ctx, mailbox, fn)
}

func (p *seamProbe) written() []seamWrite {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.writes)
}

func (p *seamProbe) taken() map[seamRead]bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return maps.Clone(p.reads)
}

// directoryInfo is what a stat answers for a directory standing where a
// file belongs.
type directoryInfo string

func (d directoryInfo) Name() string     { return string(d) }
func (directoryInfo) Size() int64        { return 0 }
func (directoryInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o700 }
func (directoryInfo) ModTime() time.Time { return time.Time{} }
func (directoryInfo) IsDir() bool        { return true }
func (directoryInfo) Sys() any           { return nil }
func (d directoryInfo) String() string   { return fs.FormatFileInfo(d) }

// remapped is the file system with the room's mailboxes moved aside: the seam
// finds them where they went, and anything that goes around the seam meets
// the file left where they were.
type remapped struct{ from, to string }

func (r remapped) at(path string) string {
	if path == r.from || strings.HasPrefix(path, r.from+string(filepath.Separator)) {
		return r.to + strings.TrimPrefix(path, r.from)
	}
	return path
}

func (r remapped) ReadFile(path string) ([]byte, error) { return osAccess{}.ReadFile(r.at(path)) }

func (r remapped) ReadDir(path string) ([]fs.DirEntry, error) { return osAccess{}.ReadDir(r.at(path)) }
func (r remapped) Stat(path string) (fs.FileInfo, error)      { return osAccess{}.Stat(r.at(path)) }
func (r remapped) WriteFile(path string, raw []byte) error {
	return osAccess{}.WriteFile(r.at(path), raw)
}

func (r remapped) Publish(path string, raw []byte) error {
	return osAccess{}.Publish(r.at(path), raw)
}

func (r remapped) EnsureDir(path string) error { return osAccess{}.EnsureDir(r.at(path)) }

func (r remapped) Remove(path string) error { return osAccess{}.Remove(r.at(path)) }

func (r remapped) Rename(from, to string) error { return osAccess{}.Rename(r.at(from), r.at(to)) }

func (r remapped) SyncDir(path string) error { return osAccess{}.SyncDir(r.at(path)) }

func (r remapped) Lock(ctx context.Context, mailbox string, fn func() error) error {
	return osAccess{}.Lock(ctx, r.at(mailbox), fn)
}

// probeDir makes an empty state directory a parallel test may use: it sets
// no environment, which only a test of its own may.
func probeDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// fresh lays a copy of the state directory src, modes and all, at dst, in
// place of whatever an earlier run left there. Every run of a scene is at
// the same path, since a note names the path of what it is about, and so
// does the name of its letter.
func fresh(t *testing.T, src, dst string) string {
	t.Helper()
	if err := os.RemoveAll(dst); err != nil {
		t.Fatal(err)
	}
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case entry.IsDir():
			return os.Mkdir(target, info.Mode().Perm())
		case info.Mode().IsRegular():
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, raw, info.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// snapshot is every file of a state directory with its content, and every
// directory, by path under it; the lock files the callers take are left out,
// and so is a stop: its records, and the letters that told main of it and
// what holds them. A stop is compared by what the calls answer.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == "." || filepath.Base(rel) == ".lock" {
			return err
		}
		if entry.IsDir() {
			tree[rel] = "<directory>"
			return nil
		}
		raw, err := os.ReadFile(path)
		tree[rel] = string(raw)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return withoutStops(tree)
}

func withoutStops(tree map[string]string) map[string]string {
	var ids []string
	for rel := range tree {
		if match := occurrenceName.FindStringSubmatch(filepath.ToSlash(rel)); match != nil && match[2] == "" {
			ids = append(ids, strings.TrimPrefix(filepath.ToSlash(rel), match[1]))
		}
	}
	dropped := map[string]bool{}
	for rel := range tree {
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) > 2 && parts[0] == "inbox" && parts[2] == stopsDir || slices.ContainsFunc(ids, func(id string) bool { return strings.Contains(rel, id) }) {
			dropped[rel] = true
		}
	}
	// A directory that held only what was left out was made for it.
	for rel, content := range tree {
		if content != "<directory>" || dropped[rel] {
			continue
		}
		held, kept := false, false
		for other := range tree {
			if strings.HasPrefix(other, rel+string(filepath.Separator)) && tree[other] != "<directory>" {
				held = held || dropped[other]
				kept = kept || !dropped[other]
			}
		}
		if held && !kept {
			dropped[rel] = true
		}
	}
	for rel := range dropped {
		delete(tree, rel)
	}
	return tree
}
