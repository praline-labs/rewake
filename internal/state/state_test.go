package state

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDirHonoursTheEnvironment(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state")
	t.Setenv(DirEnv, base)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if dir != base {
		t.Fatalf("Dir = %q, want %q", dir, base)
	}
	for _, sub := range []string{"sessions", "inbox", "sock"} {
		if _, err := os.Stat(filepath.Join(base, sub)); err != nil {
			t.Errorf("missing %s: %v", sub, err)
		}
	}
}

func TestDirRefusesRelativePath(t *testing.T) {
	t.Setenv(DirEnv, "relative/path")
	if _, err := Dir(); err == nil {
		t.Fatal("a relative state directory was accepted")
	}
}

// Messages and socket paths live in the state directory, so one that others can
// write is a way into every session. Refusing is the whole point of Verify.
func TestVerifyRefusesAWorldWritableDirectory(t *testing.T) {
	base := filepath.Join(t.TempDir(), "open")
	if err := os.MkdirAll(base, 0o777); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(base, 0o777); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	err := Verify(base)
	if err == nil {
		t.Fatal("a world-writable state directory was accepted")
	}
	if !strings.Contains(err.Error(), "chmod 700") {
		t.Errorf("refusal does not say how to fix it: %v", err)
	}
}

func TestVerifyRefusesASymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	link := filepath.Join(root, "link")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := Verify(link); err == nil {
		t.Fatal("a symlinked state directory was accepted")
	}
}

func TestWriteAtomicReplacesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")

	if err := WriteAtomic(path, []byte("first")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := WriteAtomic(path, []byte("second")); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(content) != "second" {
		t.Errorf("content = %q, want second", content)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".tmp-*"))
	if len(leftovers) != 0 {
		t.Errorf("temporary files left behind: %v", leftovers)
	}
}

func TestPublishExclusiveRefusesATakenName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.json")

	if err := PublishExclusive(path, []byte("first")); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	err := PublishExclusive(path, []byte("second"))
	if !errors.Is(err, ErrNameTaken) {
		t.Fatalf("second publish err = %v, want ErrNameTaken", err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != "first" {
		t.Errorf("content = %q, want the first writer's", content)
	}
}

func TestValidName(t *testing.T) {
	for _, name := range []string{"api", "claude-2", "web.1", "a_b"} {
		if !ValidName(name) {
			t.Errorf("%q was refused", name)
		}
	}
	for _, name := range []string{"", "Api", "with space", "../escape", strings.Repeat("a", 33)} {
		if ValidName(name) {
			t.Errorf("%q was accepted", name)
		}
	}
}

// A reader must never see a half-written file. Replacing the atomic write with a
// plain one passes every sequential test and fails this one, because a reader
// looking at the same moment catches the partial content.
func TestWriteAtomicIsNeverSeenPartially(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")

	first := []byte(strings.Repeat("a", 1<<20))
	second := []byte(strings.Repeat("b", 1<<20))
	if err := WriteAtomic(path, first); err != nil {
		t.Fatalf("write: %v", err)
	}

	stop := make(chan struct{})
	bad := make(chan int, 1)
	go func() {
		for {
			select {
			case <-stop:
				close(bad)
				return
			default:
			}
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if len(content) != len(first) {
				select {
				case bad <- len(content):
				default:
				}
				return
			}
		}
	}()

	for range 20 {
		payload := first
		if time.Now().UnixNano()%2 == 0 {
			payload = second
		}
		if err := WriteAtomic(path, payload); err != nil {
			t.Fatalf("rewrite: %v", err)
		}
	}
	close(stop)

	if size, seen := <-bad; seen {
		t.Fatalf("a reader saw %d bytes of a %d byte file", size, len(first))
	}
}

// Two processes claiming one name must not both succeed. Sequentially a check
// followed by a write looks exclusive; concurrently it is not.
func TestPublishExclusiveHasOneWinnerUnderRace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.json")

	var group sync.WaitGroup
	results := make([]error, 8)
	start := make(chan struct{})
	for index := range results {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results[index] = PublishExclusive(path, []byte(strconv.Itoa(index)))
		}()
	}
	close(start)
	group.Wait()

	winners := 0
	for _, err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrNameTaken) {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("%d of %d publishes won, want exactly 1", winners, len(results))
	}
}

func TestNameLockSerialisesClaims(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state")
	t.Setenv(DirEnv, base)
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}

	var inside, peak int
	var guard sync.Mutex
	var group sync.WaitGroup
	for range 6 {
		group.Add(1)
		go func() {
			defer group.Done()
			_ = WithNameLock(dir, "api", func() error {
				guard.Lock()
				inside++
				if inside > peak {
					peak = inside
				}
				guard.Unlock()

				time.Sleep(10 * time.Millisecond)

				guard.Lock()
				inside--
				guard.Unlock()
				return nil
			})
		}()
	}
	group.Wait()

	if peak != 1 {
		t.Fatalf("%d claimants held the name at once, want 1", peak)
	}
}
