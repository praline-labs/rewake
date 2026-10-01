package server_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// faultScenario is a call the fault test breaks at each of its steps.
type faultScenario struct {
	name string
	// prepare leaves what the call needs, makes the clean calls before it,
	// and returns its words.
	prepare func(r *rig) []string
	// acceptable says an answer is one of the call's clean answers.
	acceptable func(text string) bool
	// shows says an answer carries what the call is for.
	shows func(text string) bool
	// read, for a read, says the letter is read.
	read func(r *rig) bool
	// settled says nothing is left to finish.
	settled func(r *rig) bool
	// absent, when set, says an answer proves the call's effect absent for
	// good: a refusal, which the same words in the same turn replay.
	absent func(text string) bool
	// check is the scenario's own end state.
	check func(t *testing.T, r *rig)
}

const (
	faultLetter  = "the letter of the fault test"
	faultHeadsUp = "the heads-up of the fault test"
)

// letterID is the id of the rig's one letter.
func letterID(r *rig) string {
	r.t.Helper()
	entries, err := os.ReadDir(state.InboxPath(r.dir, "api"))
	if err != nil {
		r.t.Fatal(err)
	}
	for _, entry := range entries {
		if id, ok := strings.CutSuffix(entry.Name(), ".json"); ok {
			return id
		}
	}
	r.t.Fatal("the rig holds no letter")
	return ""
}

func letterRead(r *rig) bool { return !r.unread(letterID(r)) }

var faultScenarios = []faultScenario{
	{
		name: "a read",
		prepare: func(r *rig) []string {
			r.letter(faultLetter)
			r.nextTurn()
			return []string{"inbox"}
		},
		acceptable: func(text string) bool {
			return strings.Contains(text, faultLetter) || strings.Contains(text, "no new messages")
		},
		shows:   func(text string) bool { return strings.Contains(text, faultLetter) },
		read:    letterRead,
		settled: letterRead,
		check:   func(*testing.T, *rig) {},
	},
	{
		name: "the next part of a read",
		prepare: func(r *rig) []string {
			r.letter(strings.Repeat("данные и \"кавычки\" — строка\n", 200) + faultLetter)
			r.nextTurn()
			r.start()
			first := r.call("inbox")
			if first.result.IsError || next(first.result.text()) == nil || r.complete(first, true) != nil {
				r.t.Fatalf("the first part: %+v", first.result)
			}
			return next(first.result.text())
		},
		acceptable: func(text string) bool {
			return strings.Contains(text, "данные") || strings.Contains(text, "no new messages")
		},
		shows: func(text string) bool {
			return strings.Contains(text, "данные") || strings.Contains(text, faultLetter)
		},
		read:    letterRead,
		settled: letterRead,
		check:   func(*testing.T, *rig) {},
	},
	{
		name: "a heads-up",
		prepare: func(r *rig) []string {
			r.nextTurn()
			return []string{"send", "--notify", "--wait", "0", "web", faultHeadsUp}
		},
		acceptable: func(text string) bool { return strings.Contains(text, "pending for web") },
		shows:      func(text string) bool { return strings.Contains(text, "pending for web") },
		settled:    func(r *rig) bool { return headsUps(r) > 0 },
		absent: func(text string) bool {
			return strings.Contains(text, "so it was not taken") && !strings.Contains(text, "was published") && !retryWords.MatchString(text)
		},
		check: func(t *testing.T, r *rig) {
			if n := headsUps(r); n > 1 {
				t.Fatalf("web holds %d copies of the heads-up", n)
			}
		},
	},
}

// headsUps counts the copies of the heads-up in web's mailbox.
func headsUps(r *rig) int {
	r.t.Helper()
	count := 0
	for _, dir := range []string{state.InboxPath(r.dir, "web"), state.UnreadPath(r.dir, "web"), state.DonePath(r.dir, "web")} {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil || entry.IsDir() {
				continue
			}
			var message inbox.Message
			if json.Unmarshal(raw, &message) == nil && message.Text == faultHeadsUp && message.From == "api" {
				count++
			}
		}
	}
	return count
}
