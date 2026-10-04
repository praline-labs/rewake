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
	lettersDir   = "letters"
	doneDir      = "done"
	unreadDir    = "unread"
	awaitingDir  = "awaiting"
	answeringDir = "answering"
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

// ObservationPath is where a session's telemetry senders write: beside its
// inbox socket, one per run for the same reason, and short enough to bind.
func ObservationPath(dir, name, run string) string {
	path := filepath.Join(dir, socketsDir, name+"."+run+".obs")
	if len(path) > 103 {
		sum := sha256.Sum256([]byte(name + "\x00" + run))
		path = filepath.Join(dir, socketsDir, fmt.Sprintf("%x.obs", sum[:12]))
	}
	return path
}

// ToolConfigPath is the file a harness that takes the mail tool's server as
// a file reads it from, beside the run's sockets and removed with them.
func ToolConfigPath(dir, name, run string) string {
	return filepath.Join(dir, socketsDir, name+"."+run+".mcp.json")
}

// ContextPath is where a run's wrapper answers the mail tool's server, its
// children and its hooks (docs/mail-bridge-server.md#who-calls): beside the
// observation socket, one per run, and short enough to bind.
func ContextPath(dir, name, run string) string {
	path := filepath.Join(dir, socketsDir, name+"."+run+".ctx")
	if len(path) > 103 {
		sum := sha256.Sum256([]byte(name + "\x00" + run))
		path = filepath.Join(dir, socketsDir, fmt.Sprintf("%x.ctx", sum[:12]))
	}
	return path
}

// AuthorityAddress is where a main run's wrapper answers for the grants it
// registered (docs/grants.md#who-can-grant): an abstract unix socket, named
// by the state directory and the run. Abstract, because a file in the state
// directory is one a sandboxed worker could replace with its own listener,
// and because no path limit applies; the name is checked through its peer
// either way. Not by the session's name: the run is known before the name is
// claimed, so the wrapper binds it before its record says it is main.
func AuthorityAddress(dir, run string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(dir) + "\x00" + run))
	return fmt.Sprintf("@rewake/grant/%x", sum[:16])
}

// KeeperAddress is where a run's wrapper keeps the grants given to it, for a
// harness that applies them through a hook the harness runs (a Claude Code
// permission hook, docs/grants.md#claude-code): abstract, like
// AuthorityAddress, and checked through its peer the same way.
func KeeperAddress(dir, name, run string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(dir) + "\x00" + name + "\x00" + run))
	return fmt.Sprintf("@rewake/keep/%x", sum[:16])
}

// ControlPath is where a run takes control requests — compact, interrupt — and
// leaves its answers (docs/remote-control.md). One per run, like the sockets:
// a request written for a run that ended must not reach the next holder of the
// name. It is no socket, so its length does not matter.
func ControlPath(dir, name, run string) string {
	return filepath.Join(dir, controlDir, name+"."+run)
}

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
