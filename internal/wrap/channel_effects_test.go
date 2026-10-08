package wrap

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/state"
)

// C8, nothing is relaunched, granted or approved by the channel
// (docs/rules/channel.md). The keeper is told every kind of event the channel
// knows, with a main to tell and a heartbeat after each: all it leaves in the
// state directory is notices in the two mailboxes. No grant record, no
// permission decision, no session record is written or changed.
func TestTheChannelGrantsAndApprovesNothing(t *testing.T) {
	dir := stateDir(t)
	mainPeer(t, dir, "lead")
	k := keeperOf(t, dir, "api")
	k.begin(true, "")
	k.alive = func() bool { return true }
	before := filesUnder(t, dir)
	events := []channel.Event{
		{Kind: channel.Hello, Generation: 1},
		{Kind: channel.Closed, Generation: 1},
		{Kind: channel.HelloRefused, Descendant: true},
		{Kind: channel.HelloRefused},
		{Kind: channel.Validated},
		{Kind: channel.TimerPassed},
		{Kind: channel.ShellObserved, Class: channel.ShellReadOnly},
		{Kind: channel.Denied},
		{Kind: channel.Exited},
	}
	for i, e := range events {
		e.At = ago(time.Duration(len(events)-i) * 2 * heartbeat)
		if e.Kind == channel.Validated {
			e.Issued = e.At.Boot
		}
		k.tell(e)
		k.mu.Lock()
		k.foldRipe(ago(0), true)
		k.mu.Unlock()
		k.beat(context.Background())
	}
	mailboxes := []string{state.InboxPath(dir, "api") + "/", state.InboxPath(dir, "lead") + "/"}
	written := 0
	after := filesUnder(t, dir)
	for path, content := range after {
		if was, ok := before[path]; ok && was == content {
			continue
		}
		if !strings.HasPrefix(path, mailboxes[0]) && !strings.HasPrefix(path, mailboxes[1]) {
			t.Errorf("the channel wrote %s", path)
		}
		written++
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			t.Errorf("the channel removed %s", path)
		}
	}
	if written == 0 {
		t.Fatal("no notice was written: the keeper did nothing, and the test proves nothing")
	}
}

// filesUnder is every regular file below dir with its content.
func filesUnder(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		content, err := os.ReadFile(path)
		files[path] = string(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
