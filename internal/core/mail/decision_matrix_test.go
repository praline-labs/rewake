package mail

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	dm "github.com/praline-labs/rewake/internal/core/mail/decisionmodel"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The decision's worlds (docs/v2/stage3-decision-manifest.md), each run
// clean and then once per durable step of its clean run: killed there, and
// with that step's flush failed. What a cut leaves is whole or absent before
// anything runs again, each run is checked against the model, and every
// decision's durable marker against the flushes it stands on. The
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

// cleanOnly says the five checks run a world clean only: the chain of 40,
// each generation the same durable steps as the one before, whose cuts are
// thousands of runs of seconds each
// (docs/v2/stage3-decision-manifest.md#clean-runs-and-crash-cuts). The full
// matrix cuts every step of it too.
func cleanOnly(w dm.World, full bool) bool {
	return !full && w.Value("generations") == dm.ChainOfForty
}

// cutsOf are a world's cuts: every step killed, and each one a flush may
// fail at flushed as well.
func cutsOf(w dm.World, steps []string, full bool) []matrixCut {
	if cleanOnly(w, full) {
		return nil
	}
	var cuts []matrixCut
	for i, step := range steps {
		cuts = append(cuts, matrixCut{at: i + 1})
		if flushable(step) {
			cuts = append(cuts, matrixCut{at: i + 1, flush: true})
		}
	}
	return cuts
}

func runWorld(t *testing.T, w dm.World, full bool, play func(*testing.T, dm.World, matrixCut) []string) {
	t.Run(strings.Join(w.Values, ","), func(t *testing.T) {
		t.Parallel()
		var steps []string
		if !t.Run("clean", func(t *testing.T) {
			steps = play(t, w, matrixCut{})
			checkWritePoints(t, steps)
			checkFlushedMarkers(t, steps, matrixCut{})
		}) {
			return
		}
		for _, cut := range cutsOf(w, steps, full) {
			mode := "kill"
			if cut.flush {
				mode = "flush"
			}
			t.Run(fmt.Sprintf("%s-%02d", mode, cut.at), func(t *testing.T) {
				t.Parallel()
				got := play(t, w, cut)
				if len(got) < cut.at {
					t.Errorf("the cut at %s never came: %d steps", steps[cut.at-1], len(got))
				}
				checkFlushedMarkers(t, got, cut)
			})
		}
	})
}

// checkFlushedMarkers holds every decision's durable marker to the flushes it
// stands on (decision_durable.go): a power loss is no world here, so an
// ignored flush would leave the same outcome, and only the order shows it.
// The marker's publication begins right after the decision's file, its
// directory and its mailbox were flushed, in that order, and never right
// after one of those flushes failed.
func checkFlushedMarkers(t *testing.T, steps []string, cut matrixCut) {
	t.Helper()
	for i, step := range steps {
		path, ok := strings.CutPrefix(step, "publish-"+state.PointTemp+" ")
		if !ok || !strings.HasSuffix(path, durableSuffix) {
			continue
		}
		decision := strings.TrimSuffix(path, durableSuffix)
		flushes := []string{"sync-file " + decision, "sync-dir " + filepath.Dir(decision), "sync-dir " + filepath.Dir(filepath.Dir(decision))}
		if i < len(flushes) || !slices.Equal(steps[i-len(flushes):i], flushes) {
			t.Errorf("the marker of %s was published after %q, not right after its flushes", decision, steps[max(0, i-len(flushes)):i])
			continue
		}
		// steps[k] is step k+1, so the flushes are steps i-2 to i.
		if cut.flush && cut.at >= i-len(flushes)+1 && cut.at <= i {
			t.Errorf("the marker of %s was published right after its flush, step %d, failed", decision, cut.at)
		}
	}
}

// checkWritePoints holds a clean log to the points of every write and
// publication (state.Points): its temporary file, that file's flush, the
// placing under its name and the directory's flush, in that order, nothing
// between them — so the cuts are taken inside each write, not only around it.
func checkWritePoints(t *testing.T, steps []string) {
	t.Helper()
	writes := 0
	for i, step := range steps {
		op, path, _ := strings.Cut(step, " ")
		if op != "write" && op != "publish" {
			continue
		}
		writes++
		want := []string{op + "-" + state.PointTemp + " " + path, op + "-" + state.PointTempSync + " " + path, step, op + "-" + state.PointDirSync + " " + path}
		if i < 2 || i+2 > len(steps) || !slices.Equal(steps[i-2:i+2], want) {
			t.Errorf("%s is not cut at its points: %q", step, steps[max(0, i-2):min(len(steps), i+2)])
		}
	}
	if writes == 0 {
		t.Error("the clean run wrote nothing")
	}
}
