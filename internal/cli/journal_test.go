package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/receipt"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// headsUps are the letters with this text waiting in web's mailbox.
func headsUps(t *testing.T, dir, text string) []mail.Message {
	t.Helper()
	var found []mail.Message
	files, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "web"), "*.json"))
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var message mail.Message
		if json.Unmarshal(raw, &message) == nil && message.Text == text {
			found = append(found, message)
		}
	}
	return found
}

// A shell repeating the words of an operation still open does not run them
// again, however old that operation is: it cannot tell a lost call from a new
// one, and names the receipt that finishes it. A finished one stands in the
// way of nothing.
func TestTheShellRefersAnOpenOperationToItsReceipt(t *testing.T) {
	dir, self, _ := peerSession(t)
	record := openOperation(t, dir, self, "send", "web", "lost look", "--notify", "--wait", "0")
	record.Created = time.Now().Add(-48 * time.Hour)
	if err := receipt.Save(dir, "api", record); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run("send", "web", "lost look", "--notify", "--wait", "0")
	if code != ExitPending || !strings.Contains(out, "rewake retry "+record.Token) {
		t.Fatalf("a shell repeat of open words: %d %s", code, out)
	}
	if letters := headsUps(t, dir, "lost look"); len(letters) != 0 {
		t.Fatalf("the shell repeat published %d letters", len(letters))
	}
	if code, _, errOut := run("retry", record.Token); code == ExitFailed || code == ExitUsage {
		t.Fatalf("retry: %s", errOut)
	}
	if letters := headsUps(t, dir, "lost look"); len(letters) != 1 {
		t.Fatalf("published %d letters", len(letters))
	}
	// Finished now: the same words from the shell are a new heads-up.
	if code, out, errOut := run("send", "web", "lost look", "--notify", "--wait", "0"); code == ExitFailed || code == ExitUsage || strings.Contains(out, "not finished") {
		t.Fatalf("a shell send after the operation finished: %d %s %s", code, out, errOut)
	}
	if letters := headsUps(t, dir, "lost look"); len(letters) != 2 {
		t.Fatalf("published %d letters", len(letters))
	}
	// Shell calls alone are never joined by their words.
	for range 2 {
		if code, _, errOut := run("send", "web", "shell look", "--notify", "--wait", "0"); code == ExitFailed || code == ExitUsage {
			t.Fatalf("shell send: %s", errOut)
		}
	}
	if letters := headsUps(t, dir, "shell look"); len(letters) != 2 {
		t.Fatalf("two shell sends published %d letters", len(letters))
	}
}

// openOperation journals the words as an operation that began and was never
// finished: what a call that died before its effect leaves behind.
func openOperation(t *testing.T, dir string, self registry.Session, words ...string) receipt.Record {
	t.Helper()
	result, err := testTable().parse(words)
	if err != nil {
		t.Fatal(err)
	}
	normal := testTable().normalized(result)
	record, err := receipt.Begin(dir, self.Name, receipt.Key{Epoch: self.Epoch(), Digest: receipt.Digest(normal)},
		receipt.Record{Words: normal, Transport: receipt.Shell})
	if err != nil {
		t.Fatal(err)
	}
	return record
}
