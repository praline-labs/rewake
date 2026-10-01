package cutover

import (
	"bytes"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// The look runs over a /proc the tests describe, never the machine's own.

const (
	us       = 1000
	stamp    = "THIS-BUILD-STAMP"
	earlier  = "GO:" + rewakeMain + "\n"
	thisOne  = "GO:" + rewakeMain + "\n" + stamp
	wakeCaps = 0x800000000
)

// fixture is one process of the tree.
type fixture struct {
	uid            int
	prm, inh       uint64
	exe            string
	hiddenExe      bool
	hiddenStatus   bool
	notDumpable    bool
	env            map[string]string
	hiddenEnv      bool
	start          uint64
	zombie, noStat bool
}

type tree struct {
	t        *testing.T
	root     string
	rootOwns map[string]bool
}

func newTree(t *testing.T) *tree {
	if os.Getuid() == 0 {
		t.Skip("root reads every file, so an unreadable one cannot be described")
	}
	return &tree{t: t, root: t.TempDir(), rootOwns: map[string]bool{}}
}

func statLine(pid int, state string, start uint64) string {
	return fmt.Sprintf("%d (x) %s %s %d 0 0\n", pid, state, strings.Repeat("0 ", 18), start)
}

func (tr *tree) add(pid int, f fixture) {
	t := tr.t
	dir := filepath.Join(tr.root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if f.start == 0 {
		f.start = 100
	}
	state := "S"
	if f.zombie {
		state = "Z"
	}
	if !f.noStat {
		write(t, filepath.Join(dir, "stat"), statLine(pid, state, f.start), 0o600)
	}
	write(t, filepath.Join(dir, "status"), fmt.Sprintf("Name:\tx\nUid:\t%d\t%d\t%d\t%d\nCapInh:\t%016x\nCapPrm:\t%016x\n", f.uid, f.uid, f.uid, f.uid, f.inh, f.prm), 0o600)
	if f.hiddenStatus {
		chmod(t, filepath.Join(dir, "status"), 0)
	}
	if f.notDumpable {
		tr.rootOwns[filepath.Join(dir, "status")] = true
	}
	binary := filepath.Join(tr.root, "bin-"+strconv.Itoa(pid))
	write(t, binary, f.exe, 0o700)
	if err := os.Symlink(binary, filepath.Join(dir, "exe")); err != nil {
		t.Fatal(err)
	}
	if f.hiddenExe {
		chmod(t, binary, 0)
	}
	var env []string
	for key, value := range f.env {
		env = append(env, key+"="+value)
	}
	write(t, filepath.Join(dir, "environ"), strings.Join(env, "\x00"), 0o600)
	if f.hiddenEnv {
		chmod(t, filepath.Join(dir, "environ"), 0)
	}
}

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
}

// scanner looks at the tree as a process of us with no capabilities. Its
// build information reads the fixture's own format, "GO:<path>\n".
func (tr *tree) scanner() Scanner {
	return Scanner{
		Proc: proc.Reader{Root: tr.root}, UID: us, Stamp: []byte(stamp),
		Caps: capabilities{hasPermitted: true, hasInheritable: true},
		BuildInfo: func(r io.ReaderAt) (*buildinfo.BuildInfo, error) {
			head := make([]byte, 256)
			n, err := r.ReadAt(head, 0)
			if err != nil && !errors.Is(err, io.EOF) {
				return nil, err
			}
			path, ok := bytes.CutPrefix(head[:n], []byte("GO:"))
			if !ok {
				return nil, errors.New("not a Go executable")
			}
			path, _, _ = bytes.Cut(path, []byte("\n"))
			return &buildinfo.BuildInfo{Path: string(path)}, nil
		},
		Owner: func(path string) (int, error) {
			if tr.rootOwns[path] {
				return 0, nil
			}
			return us, nil
		},
	}
}

func (tr *tree) look() map[int]Process {
	tr.t.Helper()
	found, err := tr.scanner().Look()
	if err != nil {
		tr.t.Fatal(err)
	}
	byPID := map[int]Process{}
	for _, process := range found {
		byPID[process.PID] = process
	}
	return byPID
}

// An unreadable executable of this user is cleared only for a cause an
// earlier-build rewake cannot have: capabilities beyond the scanner's,
// permitted or inheritable, or not being dumpable. Dumpable with no such
// capability, it refuses; so does a status that cannot be read, and another
// user's process is never a writer.
func TestAnUnreadableExecutableIsClearedOnlyForAProvenCause(t *testing.T) {
	tr := newTree(t)
	tr.add(10, fixture{uid: us, prm: wakeCaps, hiddenExe: true})
	tr.add(11, fixture{uid: us, inh: wakeCaps, hiddenExe: true})
	tr.add(12, fixture{uid: us, notDumpable: true, hiddenExe: true})
	tr.add(13, fixture{uid: us, hiddenExe: true})
	tr.add(14, fixture{uid: us, hiddenStatus: true, exe: earlier})
	tr.add(15, fixture{uid: 1001, hiddenExe: true})
	found := tr.look()
	for _, pid := range []int{10, 11, 12, 15} {
		if process, ok := found[pid]; ok {
			t.Errorf("pid %d was not cleared: %+v", pid, process)
		}
	}
	if found[13].Kind != UnknownExecutable || found[14].Kind != UnknownUser || len(found) != 2 {
		t.Errorf("found %+v", found)
	}
}

// Another program and a rewake of this build are not writers; an ended
// process is not either: gone from the tree, a zombie, or replaced by another
// process under its pid while it was read.
func TestWhatIsProvenNotAWriter(t *testing.T) {
	tr := newTree(t)
	tr.add(20, fixture{uid: us, exe: "#!/bin/sh"})
	tr.add(21, fixture{uid: us, exe: "GO:example.com/other\n"})
	tr.add(22, fixture{uid: us, exe: thisOne})
	tr.add(23, fixture{uid: us, exe: earlier, noStat: true})
	tr.add(24, fixture{uid: us, exe: earlier, zombie: true})
	tr.add(25, fixture{uid: us, exe: earlier, start: 7})
	tr.add(26, fixture{uid: us, exe: earlier})
	scanner := tr.scanner()
	read := scanner.BuildInfo
	scanner.BuildInfo = func(r io.ReaderAt) (*buildinfo.BuildInfo, error) {
		// Pid 25 ends while it is read, and a new process takes its pid.
		if file, ok := r.(*os.File); ok && strings.HasSuffix(file.Name(), filepath.Join("25", "exe")) {
			write(t, filepath.Join(tr.root, "25", "stat"), statLine(25, "S", 8), 0o600)
		}
		return read(r)
	}
	found, err := scanner.Look()
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].PID != 26 || found[0].Kind != EarlierRewake {
		t.Errorf("found %+v", found)
	}
}

// An earlier-build rewake is cleared only when it belongs to another session
// by its whole address: its environment names it, or, for a wrapper, which
// carries no name, a record under its root names its pid and start. One of
// this name, one no record attributes and one whose environment cannot be
// read refuse, naming its path when the upgrade left its file on disk.
func TestAnEarlierRewakeIsClearedOnlyForAnotherSession(t *testing.T) {
	tr := newTree(t)
	stateRoot := t.TempDir()
	if err := os.Chmod(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	room, err := state.RoomDir(stateRoot, state.DefaultRoom)
	if err != nil {
		t.Fatal(err)
	}
	record := func(name string, pid int) {
		raw, _ := json.Marshal(registry.Session{Name: name, ServicePID: pid, ServiceStart: 100})
		write(t, state.SessionPath(room, name), string(raw), 0o600)
	}
	record("web", 31)
	record("api", 32)
	env := func(name string) map[string]string {
		return map[string]string{state.DirEnv: stateRoot, state.SessionEnv: name}
	}
	tr.add(30, fixture{uid: us, exe: earlier, env: env("web")})
	tr.add(31, fixture{uid: us, exe: earlier, env: map[string]string{state.DirEnv: stateRoot}})
	tr.add(32, fixture{uid: us, exe: earlier, env: map[string]string{state.DirEnv: stateRoot}})
	tr.add(33, fixture{uid: us, exe: earlier, env: env("api")})
	tr.add(34, fixture{uid: us, exe: earlier, env: map[string]string{state.DirEnv: stateRoot}})
	tr.add(35, fixture{uid: us, exe: earlier, hiddenEnv: true})
	found, err := tr.scanner().Look()
	if err != nil {
		t.Fatal(err)
	}
	blocking := map[int]Process{}
	for _, process := range Blockers(found, Address{Root: stateRoot, Room: state.DefaultRoom, Name: "api"}) {
		blocking[process.PID] = process
	}
	if len(blocking) != 4 || blocking[32].Owner == nil || blocking[33].Owner == nil || blocking[34].Owner != nil || blocking[35].Owner != nil {
		t.Fatalf("blocking %+v", blocking)
	}
	if blocking[32].Path == "" {
		t.Error("an executable left on disk was not named")
	}
	refusal := (&RefusalError{Name: "api", Blocking: Blockers(found, Address{Root: stateRoot, Room: state.DefaultRoom, Name: "api"})}).Error()
	for _, want := range []string{"pid 32, earlier rewake", "pid 34, earlier rewake", "still on disk", "launch again"} {
		if !strings.Contains(refusal, want) {
			t.Errorf("the refusal does not say %q:\n%s", want, refusal)
		}
	}
}

// The refusal tells an unknown user, an unknown executable and an earlier
// rewake apart, each with its pid and reason.
func TestTheRefusalNamesEachKind(t *testing.T) {
	refusal := (&RefusalError{Name: "api", Blocking: []Process{
		{PID: 1, Kind: UnknownUser, Detail: "a"},
		{PID: 2, Kind: UnknownExecutable, Detail: "b"},
		{PID: 3, Kind: EarlierRewake, Detail: "c"},
	}}).Error()
	for _, want := range []string{"pid 1, unknown user: a", "pid 2, unknown executable: b", "pid 3, earlier rewake: c"} {
		if !strings.Contains(refusal, want) {
			t.Errorf("missing %q in %s", want, refusal)
		}
	}
}

// The stamp is found wherever it lies, across the reads that stream the file.
func TestTheStampIsFoundAcrossReads(t *testing.T) {
	for _, at := range []int{0, 1<<20 - 3, 1 << 20, 3<<20 + 5} {
		content := append(bytes.Repeat([]byte{'x'}, at), stamp...)
		content = append(content, bytes.Repeat([]byte{'y'}, 17)...)
		if found, err := contains(bytes.NewReader(content), []byte(stamp)); err != nil || !found {
			t.Errorf("at %d: %v %v", at, found, err)
		}
	}
	if found, _ := contains(bytes.NewReader(bytes.Repeat([]byte{'x'}, 3<<20)), []byte(stamp)); found {
		t.Error("found a stamp that is not there")
	}
}
