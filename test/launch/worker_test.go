package launch_test

import (
	"slices"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/catalog"
	"github.com/praline-labs/rewake/internal/host/launch"
)

// This is the space-host command, checked against the real catalog. pi once
// classified its freshly assigned conversation id as a resume in a worktree.
func TestSpaceHostCommandForEachHarness(t *testing.T) {
	for _, h := range catalog.New().All() {
		for _, checkout := range []string{"", "true", "patch"} {
			t.Run(h.ID()+"/checkout="+checkout, func(t *testing.T) {
				worker := launch.Worker{Name: "api", Room: "room", Role: "write", Worktree: checkout, Conversation: "0f0e0d0c-0b0a-4908-8706-050403020100"}
				got, err := launch.WorkerArgs(h, worker)
				if err != nil {
					t.Fatal(err)
				}
				want := []string{"--name", "api", "--room", "room", "--write", h.ID()}
				taker, worktrees := h.(adapter.WorktreeHarness)
				if checkout != "" {
					if !worktrees {
						t.Fatal("worktree launch succeeded without an offer")
					}
					flag := taker.WorktreeFlag()
					if checkout != "true" {
						flag += "=" + checkout
					}
					want = append(want, flag)
				}
				if offer := h.Offers().Conversation; offer.Names() {
					want = append(want, offer.Flag, worker.Conversation)
				}
				if !slices.Equal(got, want) {
					t.Fatalf("space host command %v, want %v", got, want)
				}
				if checkout != "" {
					if err := taker.WorktreeRefusal(got[6:]); err != nil {
						t.Fatalf("space host command refused: %v", err)
					}
				}
			})
		}
	}
}

func TestPortableSettingsUseHarnessFlags(t *testing.T) {
	for _, h := range catalog.New().All() {
		t.Run(h.ID(), func(t *testing.T) {
			settings, ok := h.(adapter.LaunchSettingsHarness)
			if !ok {
				t.Skip("this harness has no portable model/effort settings")
			}
			effort := "--effort"
			if h.ID() == "pi" {
				effort = "--thinking"
			}
			if got, err := settings.LaunchSettings("chosen", "high", ""); err != nil || !slices.Equal(got, []string{"--model", "chosen", effort, "high"}) {
				t.Fatalf("translated settings: %v, %v", got, err)
			}
			if got, err := settings.LaunchSettings("", "", ""); err != nil || len(got) != 0 {
				t.Fatalf("unset settings became flags: %v, %v", got, err)
			}
		})
	}
}
