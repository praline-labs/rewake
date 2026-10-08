package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
)

// A session that is not main gives no grant, so an edit of its letter carries
// none, whatever the letter on disk was made to say: its recipient could
// otherwise have a grant appear in the name of a session that cannot give one.
func TestAnEditByASessionThatIsNotMainCarriesNoGrant(t *testing.T) {
	for _, sender := range []string{"write", "general"} {
		t.Run(sender, func(t *testing.T) {
			w := newGrantWorld(t, sender, "")
			if code, out, stderr := run("send", w.peer.Name, "Look at the logs", "--wait=0"); code != ExitPending && code != ExitOK {
				t.Fatalf("send: %d %s %s", code, out, stderr)
			}
			original := w.sent(t)[0]
			w.forgeLetter(t, original.ID, func(message *inbox.Message) {
				message.GrantDirs, message.GrantGit = []string{w.home}, true
			})
			if code, out, stderr := run("edit", original.ID, "Look at the logs of yesterday", "--wait=0"); code != ExitPending && code != ExitOK {
				t.Fatalf("edit: %d %s %s", code, out, stderr)
			}
			replacement, ok := w.replacementOf(t, original.ID)
			if !ok {
				t.Fatal("the edit wrote no replacement")
			}
			if inbox.CarriesGrant(replacement) || len(replacement.GrantBroad) != 0 {
				t.Fatalf("the replacement carries %v git %v", replacement.GrantDirs, replacement.GrantGit)
			}
		})
	}
}
