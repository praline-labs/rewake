package wrap

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/grantauth"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/state"
)

// A cold resume of a conversation that held a grant starts a new run, whose
// wrapper holds nothing of it (docs/grants.md#after-a-cold-resume). For a
// harness that takes grants through its own hook, the wrapper finds what the
// conversation had in the copies of the journals, asks the main that sent
// each grant to confirm it again, and keeps what is confirmed as it keeps a
// grant delivered to this run. When the launch arguments name the
// conversation, that happens before the harness starts, and the directories
// are given to it at launch; otherwise once the session's telemetry names the
// conversation, and the hook adds a directory at its first write, as it does
// for any grant.

// resumed is the grants confirmed again for the conversation a launch
// resumes, before the harness starts.
type resumed struct {
	thread string
	grants []grantauth.Restored
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
// arguments name, for a harness that takes grants through its hook.
func resumeGrants(dir, name, epoch string, adapter harness.Harness, args []string) resumed {
	resumer, ok := adapter.(harness.Resumer)
	if _, hooked := adapter.(harness.HookGranter); !ok || !hooked {
		return resumed{}
	}
	thread := resumer.ResumedConversation(args)
	if thread == "" {
		return resumed{}
	}
	out := resumed{thread: thread}
	out.grants, out.notes = confirmedAgain(dir, name, epoch, thread)
	return out
}

// confirmedAgain asks for the grants the copies name for a conversation, and
// keeps those confirmed whose directories pass this session's rules again. It
// says in a note what it restored and what it did not.
func confirmedAgain(dir, name, epoch, thread string) (confirmed []grantauth.Restored, notes []string) {
	rules := grant.CurrentEnv(state.RootForRoom(dir), harness.AllProtectedDirs()).Rules()
	for _, restored := range grantauth.Restore(dir, name, epoch, thread) {
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
			if errors.Is(err, grantauth.ErrUnreachable) {
				confirmed = append(confirmed, grantauth.Restored{Hint: restored.Hint, Err: err})
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

// keepResumed puts what was confirmed again into the keeper, as delivered to
// this run in that conversation. added says the harness was started with the
// directories. It answers whether a grant is left to ask about again: its
// main did not answer yet.
func keepResumed(keeper *grantauth.Keeper, thread string, grants []grantauth.Restored, added bool) (again bool) {
	for _, restored := range grants {
		if restored.Err != nil {
			again = true
			continue
		}
		origin := grant.Entry{Message: restored.Grant.ID, At: time.Now(), Thread: thread, From: restored.Hint.From, FromEpoch: restored.Hint.FromEpoch}
		_ = keeper.GrantFrom(origin, restored.Grant.Dirs, added)
	}
	return again
}

// followInterval is how often the wrapper looks whether the session's
// conversation changed, and asks again a main that did not answer.
const followInterval = 2 * time.Second

// followConversation restores, for each conversation the session's telemetry
// names, the grants the copies say it had: a --continue, a picker, a resume
// inside the session. Each conversation is looked at once, unless a main did
// not answer; the keeper takes a message only once.
func followConversation(ctx context.Context, dir, name, epoch string, keeper *grantauth.Keeper, conversation func() string, done string) {
	seen := map[string]bool{}
	if done != "" {
		seen[done] = true
	}
	ticker := time.NewTicker(followInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		thread := conversation()
		if thread == "" || seen[thread] {
			continue
		}
		grants, _ := confirmedAgain(dir, name, epoch, thread)
		seen[thread] = !keepResumed(keeper, thread, grants, false)
	}
}

// followResumed keeps what was confirmed again before the launch, and follows
// the session's conversation from then on.
func followResumed(ctx context.Context, dir, name, epoch string, keeper *grantauth.Keeper, restored resumed, observer harness.Observer) {
	done := ""
	if restored.thread != "" && !keepResumed(keeper, restored.thread, restored.grants, true) {
		done = restored.thread
	}
	if conversation := conversationOf(observer); conversation != nil {
		go followConversation(ctx, dir, name, epoch, keeper, conversation, done)
	}
}

// conversationOf is the conversation an observer last heard named, nil for
// one that does not follow it.
func conversationOf(observer harness.Observer) func() string {
	source, ok := observer.(harness.ThreadSource)
	if !ok {
		return nil
	}
	return func() string {
		thread, err := source.Thread()
		if err != nil {
			return ""
		}
		return thread
	}
}
