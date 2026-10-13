package mail

import (
	"fmt"
	"os"
	"strings"
	"testing"

	dm "github.com/praline-labs/rewake/internal/core/mail/decisionmodel"
)

// The decision's worlds (docs/v2/stage3-decision-manifest.md), each run
// clean and then once per durable step of its clean run: killed there, and
// with that step's flush failed. What a cut leaves is whole or absent before
// anything runs again, and each run is checked against the model. The
// five checks run the generated pair cover of every partition; with
// REWAKE_DECISION_MATRIX=full every feasible world runs.
func TestDecisionDurabilityAtEveryCut(t *testing.T) {
	plays := map[string]func(*testing.T, dm.World, matrixCut) []string{
		"decision":    playDecision,
		"resend":      playResend,
		"mailbox":     playMailbox,
		"generations": playGenerations,
	}
	full := os.Getenv(matrixEnv) == "full"
	for _, p := range dm.Partitions() {
		play, ok := plays[p.Name]
		if !ok {
			t.Fatalf("the partition %s has no play", p.Name)
		}
		worlds := p.PairCover()
		if full {
			worlds = p.All()
		}
		t.Run(p.Name, func(t *testing.T) {
			for _, w := range worlds {
				runWorld(t, w, full, play)
			}
		})
	}
}

// matrixEnv selects the whole matrix rather than its pair cover.
const matrixEnv = "REWAKE_DECISION_MATRIX"

// longChain is the clean log past which a world is a long chain of
// decisions, each generation the same durable steps as the one before.
const longChain = 100

// cutsOf are the steps a world is cut at: every one, except that the five
// checks run a long chain clean only — its cuts are thousands of runs of
// seconds each (docs/v2/stage3-decision-manifest.md#clean-runs-and-crash-cuts)
// — and the full matrix cuts every step of it too.
func cutsOf(steps []string, full bool) []int {
	if !full && len(steps) > longChain {
		return nil
	}
	cuts := make([]int, len(steps))
	for i := range cuts {
		cuts[i] = i + 1
	}
	return cuts
}

func runWorld(t *testing.T, w dm.World, full bool, play func(*testing.T, dm.World, matrixCut) []string) {
	t.Run(strings.Join(w.Values, ","), func(t *testing.T) {
		t.Parallel()
		var steps []string
		if !t.Run("clean", func(t *testing.T) { steps = play(t, w, matrixCut{}) }) {
			return
		}
		for _, at := range cutsOf(steps, full) {
			for _, flush := range []bool{false, true} {
				mode := "kill"
				if flush {
					mode = "flush"
				}
				t.Run(fmt.Sprintf("%s-%02d", mode, at), func(t *testing.T) {
					t.Parallel()
					if got := play(t, w, matrixCut{at: at, flush: flush}); len(got) < at {
						t.Errorf("the cut at %s never came: %d steps", steps[at-1], len(got))
					}
				})
			}
		}
	})
}
