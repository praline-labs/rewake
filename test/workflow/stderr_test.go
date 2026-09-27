package workflow

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// What a session prints to its standard error goes to a file in the case's
// home: the wrapper's refusals and the fixture's complaints are otherwise
// thrown away, and a session that exits 1 says nothing of why. A git worktree
// add that died on another checkout's half-written entry took a rerun with the
// capture added by hand to see (docs/research-worktree.md); the capture is here
// so the next cause of that kind is in the first red run.
//
// The file is the process's own descriptor, not a pipe the test copies from: a
// descendant holding a pipe open would decide when the session's wait returns
// (process_test.go). A green case deletes it with its directory; a red one
// keeps it, cut to stderrKept, and names each file's last line in its record.

// stderrKept bounds one kept file: its first and last halves stay, and a line
// between them says how much was cut. The evidence of a run is kept per failing
// case, and one session looping on an error should not fill the disk with it.
const stderrKept = 64 << 10

// stderrLines bounds how many files a record names; a case starts a handful
// of sessions, and the summary's excerpt has its own ceiling.
const stderrLines = 4

// captureStderr sends cmd's standard error to <dir>/<name>.stderr. The
// returned func closes this process's copy of the file once cmd has started.
func (c *Case) captureStderr(cmd *exec.Cmd, dir, name string) func() {
	path := filepath.Join(dir, name+".stderr")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		// A capture that cannot be made loses a diagnostic, not the case.
		c.Note(fmt.Sprintf("cannot capture the standard error of %s: %v", name, err))
		return func() {}
	}
	cmd.Stderr = file
	c.mu.Lock()
	c.captured = append(c.captured, path)
	c.mu.Unlock()
	return func() { _ = file.Close() }
}

// keptStderr cuts every captured file to stderrKept and returns, for the
// record, the last line of each that is not empty: the line a failure most
// often ends on.
func (c *Case) keptStderr() []string {
	c.mu.Lock()
	captured := append([]string(nil), c.captured...)
	c.mu.Unlock()
	var lines []string
	for _, path := range captured {
		last, err := cutStderr(path)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s: %v", filepath.Base(path), err))
		} else if last != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", filepath.Base(path), last))
		}
		if len(lines) == stderrLines {
			break
		}
	}
	return lines
}

// cutStderr keeps the head and the tail of a file past stderrKept, and returns
// its last line that holds anything. Only those two parts are read: the file
// being cut is the one that may be too large to read whole.
func cutStderr(path string) (string, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	size := info.Size()
	if size <= stderrKept {
		data, err := io.ReadAll(file)
		return lastLine(data), err
	}
	half := int64(stderrKept / 2)
	head, tail := make([]byte, half), make([]byte, half)
	if _, err := file.ReadAt(head, 0); err != nil {
		return "", err
	}
	if _, err := file.ReadAt(tail, size-half); err != nil {
		return "", err
	}
	var cut bytes.Buffer
	cut.Write(head)
	fmt.Fprintf(&cut, "\n… %d bytes cut by the workflow suite …\n", size-2*half)
	cut.Write(tail)
	if err := file.Truncate(0); err != nil {
		return lastLine(tail), err
	}
	_, err = file.WriteAt(cut.Bytes(), 0)
	return lastLine(tail), err
}

func lastLine(data []byte) string {
	lines := bytes.Split(bytes.TrimRight(data, " \t\r\n"), []byte("\n"))
	line := []rune(string(bytes.TrimSpace(lines[len(lines)-1])))
	if len(line) > 300 {
		return string(line[:300]) + "…"
	}
	return string(line)
}

// A red case keeps its sessions' standard error and names the last line each
// printed, in its failure and in its record.
func TestARedCaseNamesWhatItsSessionsPrinted(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "stub", Observations: []string{"never made"}})
	dir := t.TempDir()
	c.RemoveOnFinish(dir)
	session := exec.Command("sh", "-c", "echo 'rewake: starting' >&2; echo 'fatal: the cause' >&2; exit 1")
	release := c.captureStderr(session, dir, "worker")
	process, err := c.start(session, true)
	release()
	if err != nil {
		t.Fatal(err)
	}
	_ = c.wait(process)
	rec.finish()

	want := "worker.stderr: fatal: the cause"
	if len(rec.errors) != 1 || !strings.Contains(rec.errors[0], "stderr: "+want) {
		t.Errorf("the failure does not name the last line: %v", rec.errors)
	}
	if len(c.stderr) != 1 || c.stderr[0] != want {
		t.Errorf("the record carries %q", c.stderr)
	}
	if kept, err := os.ReadFile(filepath.Join(dir, "worker.stderr")); err != nil || !strings.Contains(string(kept), "rewake: starting") {
		t.Errorf("the file kept %q: %v", kept, err)
	}
}

// A green case takes its sessions' standard error away with its directory, and
// names none of it.
func TestAGreenCaseKeepsNoStderr(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "stub", Observations: []string{"made"}})
	dir := filepath.Join(t.TempDir(), "case")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	c.RemoveOnFinish(dir)
	session := exec.Command("sh", "-c", "echo 'noise' >&2")
	release := c.captureStderr(session, dir, "worker")
	process, err := c.start(session, false)
	release()
	if err != nil {
		t.Fatal(err)
	}
	_ = c.wait(process)
	c.Observed("made", "fine")
	rec.finish()

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the directory is still there: %v", err)
	}
	if len(c.stderr) != 0 || len(rec.errors) != 0 {
		t.Errorf("stderr %q, errors %v", c.stderr, rec.errors)
	}
}

// A kept file past stderrKept keeps its head and tail, says how much went, and
// its last line is still the one named.
func TestAKeptStderrIsCutToItsHeadAndTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loud.stderr")
	body := "the first line\n" + strings.Repeat("again and again\n", 20000) + "the last line\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	last, err := cutStderr(path)
	if err != nil || last != "the last line" {
		t.Fatalf("last %q: %v", last, err)
	}
	cut, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cut) > stderrKept+100 || !strings.HasPrefix(string(cut), "the first line\n") ||
		!strings.HasSuffix(string(cut), "the last line\n") || !strings.Contains(string(cut), "bytes cut by the workflow suite") {
		t.Errorf("cut to %d bytes: %q … %q", len(cut), cut[:40], cut[len(cut)-40:])
	}
}
