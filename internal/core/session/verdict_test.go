package session

import (
	"testing"
	"time"
)

// The verdict is what every reader shows; a row here is the whole of what
// one snapshot reads as, in `rewake list`, herdr and the shell.
func TestTheVerdictOnASnapshot(t *testing.T) {
	yes, dialog := true, "trust this folder"
	type shape struct {
		activity           string
		waiting            string
		dialog, compacting bool
		stale, unheard     bool
		staleRun           bool
	}
	for _, row := range []struct {
		name string
		shape
		want Verdict
	}{
		{"idle", shape{activity: "idle"}, Verdict{"idle", "idle", "", false, MarkOff}},
		{"working", shape{activity: "working"}, Verdict{"working", "working", "", false, MarkSteady}},
		{"compacting while idle", shape{activity: "idle", compacting: true}, Verdict{"compacting", "compacting", "", false, MarkSteady}},
		{"permission", shape{activity: "working", waiting: WaitingPermission}, Verdict{ActivityPermission, ActivityPermission, BlockedPermission, false, MarkBlink}},
		{"question over a compaction", shape{activity: "working", compacting: true, waiting: WaitingQuestion}, Verdict{ActivityQuestion, ActivityQuestion, BlockedQuestion, false, MarkBlink}},
		{"dialog while idle", shape{activity: "idle", dialog: true}, Verdict{ActivityDialog, ActivityDialog, BlockedDialog, false, MarkBlink}},
		{"another word", shape{activity: "not_loaded"}, Verdict{"unknown", "unknown", "", false, MarkOff}},
		{"never reported", shape{}, Verdict{"unknown", "unknown", "", false, MarkOff}},
		{"interruptions unheard", shape{activity: "working", unheard: true}, Verdict{"working", "working; interruptions unheard", "", false, MarkSteady}},
		{"stale idle", shape{activity: "idle", stale: true}, Verdict{"idle", "idle (stale)", "", true, MarkOff}},
		{"stale working", shape{activity: "working", stale: true}, Verdict{"working", "working (stale)", "", true, MarkBlink}},
		{"stale compacting", shape{activity: "working", compacting: true, stale: true}, Verdict{"compacting", "compacting (stale)", "", true, MarkBlink}},
		{"stale permission", shape{activity: "idle", waiting: WaitingPermission, stale: true}, Verdict{ActivityPermission, ActivityPermission + " (stale)", BlockedPermission, true, MarkBlink}},
		{"stale, unheard", shape{activity: "working", unheard: true, stale: true}, Verdict{"working", "working; interruptions unheard (stale)", "", true, MarkBlink}},
		{"stale unknown word", shape{activity: "not_loaded", stale: true}, Verdict{"unknown", "unknown (stale)", "", true, MarkOff}},
		{"run gone, activity kept", shape{activity: "working", staleRun: true}, Verdict{"working", "working (stale)", "", true, MarkBlink}},
	} {
		s := Unknown("e")
		if row.activity != "" {
			activity, now := row.activity, time.Now()
			s.Activity, s.ActivityAt = &activity, &now
		}
		s.Fresh, s.ActivityFresh = !row.stale && !row.staleRun, !row.stale
		s.Waiting = row.waiting
		if row.dialog {
			s.Dialog = &dialog
		}
		if row.compacting {
			s.Compacting = &yes
		}
		if row.unheard {
			s.Interruptions = InterruptionsUnobserved
		}
		if got := s.Judge(); got != row.want {
			t.Errorf("%s: got %+v, want %+v", row.name, got, row.want)
		}
	}
}
