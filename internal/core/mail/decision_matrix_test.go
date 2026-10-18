package mail

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	dm "github.com/praline-labs/rewake/internal/core/mail/decisionmodel"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Four fixed worlds retain the recovery checks at every durable cut,
// including the flush ordering that a completed run alone cannot prove.
func TestDecisionDurabilityAtEveryCut(t *testing.T) {
	plays := map[string]func(*testing.T, dm.World, matrixCut) []string{
		"decision":    playDecision,
		"resend":      playResend,
		"mailbox":     playMailbox,
		"generations": playGenerations,
	}
	for _, w := range dm.Worlds() {
		play, ok := plays[w.Partition]
		if !ok {
			t.Fatalf("the world %s has no play", w.Name)
		}
		runWorld(t, w, play)
	}
}

// cutsOf are a world's cuts: every step killed, and each one a flush may
// fail at flushed as well.
func cutsOf(steps []string) []matrixCut {
	var cuts []matrixCut
	for i, step := range steps {
		cuts = append(cuts, matrixCut{at: i + 1})
		if flushable(step) {
			cuts = append(cuts, matrixCut{at: i + 1, flush: true})
		}
	}
	return cuts
}

func runWorld(t *testing.T, w dm.World, play func(*testing.T, dm.World, matrixCut) []string) {
	t.Run(w.Name, func(t *testing.T) {
		t.Parallel()
		var steps []string
		if !t.Run("clean", func(t *testing.T) {
			steps = play(t, w, matrixCut{})
			checkWritePoints(t, steps)
			checkFlushedMarkers(t, steps, matrixCut{})
		}) {
			return
		}
		for _, cut := range cutsOf(steps) {
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
