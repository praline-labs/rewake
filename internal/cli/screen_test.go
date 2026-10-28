package cli

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

const screenFrame = `{"cols":6,"rows":2,"cursor":{"x":1,"y":0},"lines":[[{"t":"ab","w":2,"fg":"#ff0000","a":1},{"t":"世","w":2}],[{"t":"x","w":1}]]}`

// listen answers every client of a socket with reply after reading its
// first line or byte, and records what it read.
func listen(t *testing.T, path string, line bool, reply string) <-chan string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	asked := make(chan string, 4)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			var got string
			if line {
				got, _ = bufio.NewReader(conn).ReadString('\n')
			} else {
				b := make([]byte, 1)
				_, _ = conn.Read(b)
				got = string(b)
			}
			select {
			case asked <- got:
			default:
			}
			_, _ = conn.Write([]byte(reply + "\n"))
			_ = conn.Close()
		}
	}()
	return asked
}

func screenRoom(t *testing.T) string {
	t.Helper()
	// A unix socket's path is short, and a test's own folder is long.
	short, err := os.MkdirTemp("", "scr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	t.Setenv(state.DirEnv, short)
	t.Setenv(state.RoomEnv, "")
	t.Setenv(state.SessionEnv, "")
	t.Setenv(epochEnv, "")
	return buildtest.Current(t)
}

// A worker's window comes from the shell: the request names the space, the
// answer is drawn into the text and the PNG, and the file is the caller's.
func TestScreenTakesTheShellsWindowAsTextAndPNG(t *testing.T) {
	dir := screenRoom(t)
	writeSpaceFor(t, dir, "helper")
	asked := listen(t, state.SocketPath(dir, "shell", ""), true, screenFrame)
	out := filepath.Join(t.TempDir(), "helper.png")

	code, stdout, stderr := run("screen", "helper-agent", "--png", out, "--text", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d: %s %s", code, stdout, stderr)
	}
	if got := <-asked; got != `{"pane":"`+strings.Repeat("a1", 16)+`"}`+"\n" {
		t.Fatalf("request = %q", got)
	}
	var view screenView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatal(err)
	}
	if view.Source != "shell" || view.Cols != 6 || view.PNG != out || strings.Join(view.Text, "|") != "ab世|x" {
		t.Fatalf("view = %+v", view)
	}
	if data, err := os.ReadFile(out); err != nil || !strings.HasPrefix(string(data), "\x89PNG") {
		t.Fatalf("picture: %v", err)
	}
}

func TestScreenOfMainAsksForMain(t *testing.T) {
	dir := screenRoom(t)
	asked := listen(t, state.SocketPath(dir, "shell", ""), true, screenFrame)
	if code, out, _ := run("screen", "main"); code != ExitOK || !strings.Contains(out, "ab世") {
		t.Fatalf("exit %d: %q", code, out)
	}
	if got := <-asked; got != `{"pane":"main"}`+"\n" {
		t.Fatalf("request = %q", got)
	}
}

// With no shell a worker's pane alone comes from its space host, marked so;
// main has no host, and is refused.
func TestScreenWithoutAShellFallsBackToTheSpaceHost(t *testing.T) {
	dir := screenRoom(t)
	writeSpaceFor(t, dir, "helper")
	asked := listen(t, state.SpaceSocketPath(dir, strings.Repeat("a1", 16)), false, screenFrame)

	code, out, errOut := run("screen", "helper")
	if code != ExitOK || !strings.Contains(out, "pane alone, from its space host") || !strings.Contains(out, "ab世") {
		t.Fatalf("exit %d: %q %q", code, out, errOut)
	}
	if got := <-asked; got != "s" {
		t.Fatalf("mode = %q", got)
	}
	if code, _, errOut := run("screen", "main"); code != ExitFailed || !strings.Contains(errOut, "no shell is running") {
		t.Fatalf("main without a shell: %d %q", code, errOut)
	}
}

func TestScreenRefusesAnUnknownName(t *testing.T) {
	screenRoom(t)
	if code, _, errOut := run("screen", "nobody"); code != ExitUsage || !strings.Contains(errOut, "nobody") {
		t.Fatalf("exit %d: %q", code, errOut)
	}
}

// The picture is written outside rewake's state, not through a link, and not
// over a file that is not a regular one; a regular file is replaced.
func TestScreenWritesOnlyWhereItMay(t *testing.T) {
	dir := screenRoom(t)
	listen(t, state.SocketPath(dir, "shell", ""), true, screenFrame)
	base, err := state.Base()
	if err != nil {
		t.Fatal(err)
	}
	plain := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(plain, link); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(plain, "target")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	fileLink := filepath.Join(plain, "file-link.png")
	if err := os.Symlink(target, fileLink); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"inside the state":     filepath.Join(dir, "picture.png"),
		"in the base":          filepath.Join(base, "picture.png"),
		"through a link":       filepath.Join(link, "picture.png"),
		"onto a link":          fileLink,
		"onto a folder":        plain,
		"in a missing folder":  filepath.Join(plain, "missing", "picture.png"),
		"inside via a dot-dot": filepath.Join(plain, "..", filepath.Base(dir), "x.png"),
	} {
		if code, _, errOut := run("screen", "main", "--png", path); code == ExitOK {
			t.Errorf("%s: written (%s)", name, errOut)
		}
	}
	if data, _ := os.ReadFile(target); string(data) != "keep" {
		t.Fatalf("the link's target was overwritten: %q", data)
	}
	existing := filepath.Join(plain, "old.png")
	if err := os.WriteFile(existing, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("screen", "main", "--png", existing); code != ExitOK {
		t.Fatalf("a regular file is replaced: %d %s", code, errOut)
	}
	if data, _ := os.ReadFile(existing); !strings.HasPrefix(string(data), "\x89PNG") {
		t.Fatal("the regular file was not replaced")
	}
}

// writeSpaceFor writes an open space record named name, its worker
// helper-agent, by hand.
func writeSpaceFor(t *testing.T, dir, name string) {
	t.Helper()
	start, err := proc.Default.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	boot, err := registry.CurrentBoot()
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a1", 16)
	record := `{"id": "` + id + `", "name": "` + name + `", "state": "open",
	"main": {"name": "lead-one", "run": "` + registry.RunEpoch(os.Getpid(), start, boot) + `", "pidNamespace": "` + proc.Namespace() + `"},
	"worker": {"harness": "agent", "name": "` + name + `", "role": "write", "run": "5151.1800"},
	"host": "6161.1750", "openedAt": "2026-10-13T12:00:00Z"}`
	if err := os.MkdirAll(state.SpacesPath(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.SpacePath(dir, id), []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
}
