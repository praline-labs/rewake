package grant

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A directory in a shared temporary directory is refused outright: a
// sandboxed worker can write there, and so swap it for a link once granted.
func TestATemporaryDirectoryIsNeverGranted(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "tmp")
	if err := os.MkdirAll(filepath.Join(temp, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	rules := Env{Temp: []string{temp}}.Rules()
	for _, path := range []string{temp, filepath.Join(temp, "work")} {
		err := rules.Check(path, path, false)
		var refusal *Refusal
		if !errors.As(err, &refusal) || refusal.Code != 2 || !strings.Contains(refusal.Message, "temporary directory") {
			t.Errorf("%s: %v", path, err)
		}
	}
	if err := rules.Check(root, root, false); err == nil {
		t.Error("a directory holding a temporary one was granted")
	}
}

// A directory where a live session works, or one holding it, is broad: it
// carries that session's own configuration. Confirmed when sent, it is not
// refused at delivery once that session has ended.
func TestALiveSessionsDirectoryIsBroad(t *testing.T) {
	root := t.TempDir()
	session := filepath.Join(root, "work", "checkout")
	if err := os.MkdirAll(filepath.Join(session, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	rules := Env{Sessions: []string{session}}.Rules()
	for _, path := range []string{session, filepath.Dir(session)} {
		if err := rules.Check(path, path, false); err == nil || !strings.Contains(err.Error(), "live rewake session") {
			t.Errorf("%s: %v", path, err)
		}
		if err := rules.Check(path, path, true); err != nil {
			t.Errorf("%s confirmed: %v", path, err)
		}
	}
	inside := filepath.Join(session, "sub")
	if err := rules.Check(inside, inside, false); err != nil {
		t.Errorf("a directory inside a session's: %v", err)
	}
	ended := Env{}.Rules()
	if err := ended.Recheck(session, true); err != nil {
		t.Errorf("a confirmation refused at delivery after its session ended: %v", err)
	}
	if err := ended.Check(session, session, true); err == nil {
		t.Error("a call confirmed a directory that is not broad")
	}
}
