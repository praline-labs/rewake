package state

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
)

// Where each thing of a room lives: records, mailboxes, sockets, and the
// abstract addresses a run's wrapper answers at.

const (
	sessionsDir  = "sessions"
	inboxDir     = "inbox"
	socketsDir   = "sock"
	controlDir   = "control"
	runDir       = "run"
	lettersDir   = "letters"
	doneDir      = "done"
	unreadDir    = "unread"
	awaitingDir  = "awaiting"
	answeringDir = "answering"
	spacesDir    = "spaces"
)

// SessionPath is the record of one session.
func SessionPath(dir, name string) string {
	return filepath.Join(dir, sessionsDir, name+".json")
}

// SessionsPath is the directory holding every session record.
func SessionsPath(dir string) string { return filepath.Join(dir, sessionsDir) }

// InboxesPath is the directory holding every mailbox of the room.
func InboxesPath(dir string) string { return filepath.Join(dir, inboxDir) }

// InboxPath is the mailbox of one session.
func InboxPath(dir, name string) string { return filepath.Join(dir, inboxDir, name) }

// DonePath holds messages that have been read or refused, for diagnosis.
func DonePath(dir, name string) string { return filepath.Join(dir, inboxDir, name, doneDir) }

// UnreadPath holds messages the session has been told about and has not read
// yet. The harness is handed a notice, not the text: the agent fetches the text
// itself, so it knows the message came through a tool and not from its user.
func UnreadPath(dir, name string) string { return filepath.Join(dir, inboxDir, name, unreadDir) }

// AnsweringPath holds one mark per question a send in this session is blocked
// on. While a mark is fresh, the answer is handed to that send rather than
// announced to the agent.
func AnsweringPath(dir, name string) string {
	return filepath.Join(dir, inboxDir, name, answeringDir)
}

// AwaitingPath lists the sessions whose messages this session has read since its
// last turn ended. Each of them is told when that turn ends.
func AwaitingPath(dir, name string) string {
	return filepath.Join(dir, inboxDir, name, awaitingDir)
}

// ControlPath is where a run takes control requests — compact, interrupt — and
// leaves its answers (docs/remote-control.md). One per run, like the sockets:
// a request written for a run that ended must not reach the next holder of the
// name. It is no socket, so its length does not matter.
func ControlPath(dir, name, run string) string {
	return filepath.Join(dir, controlDir, name+"."+run)
}

// ControlsPath is the directory holding every run's control directory.
func ControlsPath(dir string) string { return filepath.Join(dir, controlDir) }

// RunPath is the directory of one run (docs/v2/stage3-rundir.md):
// what an adapter writes for that run alone, handed to it at launch and
// removed with the run. It is no socket, so its length does not matter.
func RunPath(dir, name, run string) string {
	return filepath.Join(dir, runDir, name+"."+run)
}

// RunsPath is the directory holding every run's directory.
func RunsPath(dir string) string { return filepath.Join(dir, runDir) }

// SpacesPath is the directory holding every space record of the room
// (docs/v2/stage4-space.md#1-the-pieces-and-where-they-live).
func SpacesPath(dir string) string { return filepath.Join(dir, spacesDir) }

// SpacePath is the record of one space, by its id: a name is reused once its
// space is closed, while the closed record stays for what a restore reads.
func SpacePath(dir, id string) string { return filepath.Join(dir, spacesDir, id+".json") }

// SpaceSocketPath is where a space host serves its worker's terminal to
// rewake attach, by the space's id. A space id has no dot, so it never meets
// a session's socket, whose run does.
func SpaceSocketPath(dir, id string) string { return SocketPath(dir, "space", id) }

// LettersPath holds the compactions a main asked for with rewake compact whose
// letter has not gone yet (docs/remote-control.md). It is kept by name, not by
// run: the next run of that main closes what an earlier one left open.
func LettersPath(dir, name string) string { return filepath.Join(dir, lettersDir, name) }

// SocketPath is where the wrapper asks a harness to put its inbox socket. It is
// kept short: a unix socket path may not exceed 103 bytes.
//
// The path carries the run as well as the name. A name changes hands, and a
// socket shared by every run of it could be removed by a wrapper on its way out
// just after the next run had bound it; a path of its own belongs to one run.
func SocketPath(dir, name, run string) string {
	basename := name
	if run != "" {
		basename += "." + run
	}
	path := filepath.Join(dir, socketsDir, basename+".sock")
	if len(path) > 103 {
		sum := sha256.Sum256([]byte(name + "\x00" + run))
		path = filepath.Join(dir, socketsDir, fmt.Sprintf("%x.sock", sum[:12]))
	}
	return path
}
