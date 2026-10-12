package host

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/grant"
	"github.com/praline-labs/rewake/internal/core/grant/authority"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A cold resume of a conversation that held a grant starts a new run, whose
// wrapper holds nothing of it (docs/grants.md#after-a-cold-resume). For a
// harness that takes directory grants and names the conversation a launch
// resumes, the wrapper finds what the conversation had in the copies of the
// journals, asks the main that sent each grant to confirm it again, and gives
// the harness what is confirmed at launch.
//
// Code only tests reach from S9: the permission hook that applied and took
// back these grants, and the keeper that fed it, left with the hooks,
// and no adapter is both a Resumer and offers Permissions.Dir. Stage 6 designs the
// Permissions capability (docs/v2/design-api.md#permissions), which applies a
// grant to a run and withdraws it, and decides there how a resumed run's
// grants are kept.

// resumed is the grants confirmed again for the conversation a launch
// resumes, before the harness starts.
type resumed struct {
	thread string
	grants []authority.Restored
	notes  []string
}

// dirs are the directories the harness is started with.
func (r resumed) dirs() []string {
	var dirs []string
	for _, restored := range r.grants {
		dirs = append(dirs, restored.Grant.Dirs...)
	}
	return dirs
}

// resumeGrants confirms again the grants of the conversation the launch
// arguments name, for a harness that takes directory grants.
func resumeGrants(dir, name, epoch string, h adapter.Harness, args []string, protected adapter.Protected) resumed {
	resumer, ok := h.(adapter.Resumer)
	if !ok || !h.Offers().Permissions.Dir {
		return resumed{}
	}
	thread := resumer.ResumedConversation(args)
	if thread == "" {
		return resumed{}
	}
	out := resumed{thread: thread}
	out.grants, out.notes = confirmedAgain(dir, name, epoch, thread, protected)
	return out
}

// confirmedAgain asks for the grants the copies name for a conversation, and
// keeps those confirmed whose directories pass this session's rules again. It
// says in a note what it restored and what it did not.
func confirmedAgain(dir, name, epoch, thread string, protected adapter.Protected) (confirmed []authority.Restored, notes []string) {
	rules := grant.CurrentEnv(state.RootForRoom(dir), protected.Dirs, protected.Names).Rules()
	for _, restored := range authority.Restore(dir, name, epoch, thread) {
		err := restored.Err
		if err == nil {
			for _, directory := range restored.Grant.Dirs {
				if err = rules.Recheck(directory, slices.Contains(restored.Grant.Broad, directory)); err != nil {
					break
				}
			}
		}
		if err != nil {
			notes = append(notes, fmt.Sprintf("the grant of task %s is not restored after the resume: %v", restored.Hint.Message, err))
			if errors.Is(err, authority.ErrUnreachable) {
				confirmed = append(confirmed, authority.Restored{Hint: restored.Hint, Err: err})
			}
			continue
		}
		if len(restored.Grant.Dirs) == 0 {
			continue
		}
		notes = append(notes, fmt.Sprintf("restored after the resume, confirmed again by %s: write %s", restored.Hint.From, strings.Join(restored.Grant.Dirs, ", ")))
		confirmed = append(confirmed, restored)
	}
	return confirmed, notes
}
