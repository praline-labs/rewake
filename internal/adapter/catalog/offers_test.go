package catalog

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// launchRequest is a launch of a role that opens spaces, in a short private directory, so no
// socket path the adapter derives passes the limit a unix socket allows.
func launchRequest(t *testing.T) adapter.LaunchRequest {
	t.Helper()
	dir, err := os.MkdirTemp("", "rw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("HOME", dir)
	run := filepath.Join(dir, "run")
	if err := os.Mkdir(run, 0o700); err != nil {
		t.Fatal(err)
	}
	request := adapter.LaunchRequest{
		Name: "api", Dir: dir, RoomDir: dir, Room: "default", Epoch: "1-1",
		Socket: filepath.Join(dir, "api.sock"), RunDir: run,
	}
	// A role that opens spaces, set field by field: the catalog sits below
	// the core and names none of its roles (the import rule).
	request.Role.ID, request.Role.Spaces, request.Role.Silent = "main", true, true
	return request
}

// Every harness answers whether a launch can name its conversation, either
// way: a flag, taken once and carried by Launch with rewake's id, or the
// reason there is none, and then Launch refuses an id rather than drop it.
func TestEveryHarnessAnswersWhetherItNamesItsConversation(t *testing.T) {
	const id = "0f6f2d55-1c3a-4c43-9a11-6c1f4bd1e0c2"
	for _, h := range New().All() {
		t.Run(h.ID(), func(t *testing.T) {
			answer := h.Offers().Conversation
			if (answer.Flag == "") == (answer.Unnamed == "") {
				t.Fatalf("the conversation offer %+v states neither or both: a flag, or why there is none", answer)
			}
			request := launchRequest(t)
			request.Conversation = id
			plan, err := h.Launch(request)
			if !answer.Names() {
				if err == nil {
					t.Fatal("a launch that names no conversation took rewake's id without a word")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.ContainsFunc(h.SingleUseFlags(), func(flag adapter.Flag) bool {
				return flag.TakesValue && slices.Contains(flag.Spellings, answer.Flag)
			}) {
				t.Fatalf("%s is not among the flags taken once with a value", answer.Flag)
			}
			args := adapter.BeforeTerminator(plan.Args)
			at := slices.Index(args, answer.Flag)
			if at < 0 || at+1 >= len(args) || args[at+1] != id {
				t.Fatalf("the launch's arguments %q do not carry %s %s", plan.Args, answer.Flag, id)
			}
		})
	}
}

// Spaces are never planned: a harness that offers them has its main run
// prove them, so its plan carries what reports the live set.
func TestEveryHarnessThatOffersSpacesHasItsRunProveThem(t *testing.T) {
	for _, h := range New().All() {
		if !h.Offers().Spaces {
			continue
		}
		t.Run(h.ID(), func(t *testing.T) {
			plan, err := h.Launch(launchRequest(t))
			if err != nil {
				t.Fatal(err)
			}
			if plan.Live == nil {
				t.Fatalf("spaces are offered and the main run reports no live set: they could never become live (notes %q)", plan.Notes)
			}
		})
	}
}
