package mail

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// withdrawNow withdraws under the recipient's mailbox lock, as the command does.
func withdrawNow(t *testing.T, dir string, message Message, replacement *Message) (Withdrawal, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var result Withdrawal
	err := state.WithMailboxLock(ctx, dir, message.To, func() error {
		var err error
		result, err = Withdraw(dir, message, replacement)
		return err
	})
	return result, err
}

func TestSentMatchingTakesUniquePrefixesOfThisRunOnly(t *testing.T) {
	dir := stateDir(t)
	put := func(id string, from, epoch string, kind Kind) Message {
		m := Message{ID: id, From: from, FromEpoch: epoch, To: "api", ToEpoch: "api-epoch", Kind: kind, Text: "x", CreatedAt: time.Now()}
		if err := Put(dir, m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	put("1790000000000000001-aaaa11112222", "web", "web-epoch", Task)
	put("1790000000000000002-aaaa33334444", "web", "web-epoch", Note)
	put("1790000000000000003-bbbb55556666", "web", "old-epoch", Task)
	put("1790000000000000004-cccc77778888", "web", "web-epoch", Finished)
	cases := map[string]int{
		"1790000000000000001-aaaa11112222": 1,
		"aaaa1":                            1,
		"aaaa":                             2,
		"1790000000000000002":              1,
		"bbbb":                             0, // an earlier run's
		"cccc":                             0, // a report
		"179":                              0, // too short
	}
	for reference, want := range cases {
		if got := must(SentMatching(dir, "web", "web-epoch", reference)); len(got) != want {
			t.Errorf("%s: %d matches, want %d", reference, len(got), want)
		}
	}
}
