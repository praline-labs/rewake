package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// An attempt that is neither the first nor shown to run in its operation's
// turn is mail.VerdictUnproven; the first attempt, and one in its own turn, mark.
func TestAnAttemptNotProvenInItsTurnIsMarkUnproven(t *testing.T) {
	dir, self, _ := peerSession(t)
	at := markAt
	for _, test := range []struct {
		scope mail.AttemptScope
		want  mail.MarkVerdict
	}{
		{mail.AttemptScope{}, mail.VerdictUnproven},
		{mail.AttemptScope{First: true}, mail.VerdictWrite},
		{mail.AttemptScope{InOwnTurn: true}, mail.VerdictWrite},
	} {
		got, err := mail.JudgeMark(dir, self, self.Epoch(), "mark-file", at, test.scope, 0)
		if err != nil || got != test.want {
			t.Fatalf("%+v: %v %v, want %v", test.scope, got, err, test.want)
		}
	}
	// A later start proves the turn ended, whoever asks.
	if got, _ := mail.JudgeMark(dir, self, self.Epoch(), "mark-file", at, mail.AttemptScope{}, at+1); got != mail.VerdictTurnEnded {
		t.Fatalf("a later start: %v", got)
	}
}
