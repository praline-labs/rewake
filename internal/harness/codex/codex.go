/*
Package codex runs Codex as a rewake session and delivers messages to it through
the message queue every Codex process polls.

Two facts shape this adapter. Codex has no flag that names a session at startup,
so the thread id is read from the lock file the process holds open. And delivery
goes through `codex queue`, which every Codex process picks up within about ten
seconds — including a plain TUI, with no daemon and no extra flags.
*/
package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// ID is the launch command and the harness field of a session record.
const ID = "codex"

// configFlag layers a configuration value over the user's file for one launch.
const configFlag = "-c"

// introKey carries the briefing. Codex has no key that appends to the user's
// instructions, so this one replaces them and the wrapper concatenates.
const introKey = "developer_instructions"

// queueTimeout bounds one call to codex queue. It talks to a local database, so
// a call that takes longer than this is stuck, not slow.
const queueTimeout = 15 * time.Second

// pollNotice is said in the result: delivery is not instant, and a sender that
// does not hear this assumes silence means failure.
const pollNotice = "codex checks its queue about every ten seconds"

type codexHarness struct{}

// New returns the Codex harness.
func New() harness.Harness { return codexHarness{} }

func (codexHarness) ID() string    { return ID }
func (codexHarness) Title() string { return "Codex" }

func (codexHarness) Summary() string {
	return "Start Codex as a rewake session. Messages reach it within about ten seconds."
}

func (codexHarness) Examples() []string {
	return []string{
		"rewake codex",
		"rewake --name web codex --model gpt-5.6-terra",
	}
}

func (codexHarness) Notes() []string {
	return []string{
		"Codex polls for queued messages every ten seconds, so delivery is not instant; the send command says so in its result.",
		"A Codex session that has not exchanged a single message yet cannot accept one: such a message stays pending and lands after its first turn.",
		"Arguments after the harness name are passed to codex untouched, with one exception: a --help written first asks rewake for this page instead of starting the harness.",
	}
}

func (codexHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	args := append([]string{}, request.Args...)
	home := Home()

	if request.Intro && !hasConfigKey(args, introKey) {
		instructions, err := introValue(home, request.Name)
		if err != nil {
			return harness.LaunchPlan{}, err
		}
		args = harness.AddFlags(args, configFlag, introKey+"="+quoteTOML(instructions))
	}

	// The state directory is under /tmp, which the sandbox may write to by
	// default. A configuration that excludes /tmp would leave the agent unable
	// to send anything, so the directory joins the writable roots without
	// disturbing the ones already there.
	if roots, needed := writableRoots(home, request.Dir); needed {
		args = harness.AddFlags(args, configFlag, "sandbox_workspace_write.writable_roots="+quoteTOMLArray(roots))
	}

	return harness.LaunchPlan{
		Command:   "codex",
		Args:      args,
		Env:       harness.SessionEnv(request, nil),
		CodexHome: home,
	}, nil
}

func (codexHarness) Deliver(ctx context.Context, session registry.Session, message inbox.Message) inbox.Result {
	home := session.CodexHome
	if home == "" {
		home = Home()
	}

	thread, err := CurrentThread(session.HarnessPID, home)
	if err != nil {
		return inbox.Result{State: inbox.Pending, Detail: err.Error()}
	}

	callCtx, cancel := context.WithTimeout(ctx, queueTimeout)
	defer cancel()

	output, err := queue(callCtx, home, thread, harness.MessageText(message))
	if err == nil {
		return inbox.Result{State: inbox.Delivered, Via: "codex queue", Detail: pollNotice}
	}
	return classify(output, err)
}

// queue is the call to the Codex CLI, replaceable in tests.
var queue = func(ctx context.Context, home, thread, text string) (string, error) {
	command := exec.CommandContext(ctx, "codex", "queue", "--thread", thread, "--message", text)
	command.Env = append(os.Environ(), "CODEX_HOME="+home)
	output, err := command.CombinedOutput()
	return string(output), err
}

// classify turns a failed codex queue call into a result the sender can act on.
func classify(output string, err error) inbox.Result {
	text := strings.TrimSpace(output)
	switch {
	case strings.Contains(text, "no rollout found"):
		// The thread exists but holds no conversation: Codex creates the file
		// behind it only once a message has been exchanged. Waiting is right —
		// the message lands after the session's first turn.
		return inbox.Result{
			State:  inbox.Pending,
			Detail: "this codex session has not exchanged a message yet; delivery happens after its first turn",
		}
	case strings.Contains(text, "No active session found"):
		return inbox.Result{
			State:  inbox.Failed,
			Detail: "codex no longer knows this thread: " + firstLine(text),
		}
	case text != "":
		return inbox.Result{State: inbox.Failed, Detail: "codex queue refused the message: " + firstLine(text)}
	default:
		return inbox.Result{State: inbox.Failed, Detail: "codex queue failed: " + err.Error()}
	}
}

func firstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	return text
}

// lockDir holds one file per thread a Codex process is writing to.
const lockDir = "thread-writer-locks"

// CurrentThread returns the thread a running Codex session is writing to.
//
// Codex publishes this nowhere else: no flag assigns an id, and none is printed
// at startup. The process does hold a lock file named after the thread open from
// its first second, and after /new it holds the old one and the new one at once
// — so the answer is the most recently touched of them, not the only one.
func CurrentThread(harnessPID int, home string) (string, error) {
	if harnessPID == 0 {
		return "", fmt.Errorf("the codex process is not recorded yet")
	}
	prefix := filepath.Join(home, lockDir) + string(filepath.Separator)

	processes, err := proc.Descendants(harnessPID)
	if err != nil {
		return "", fmt.Errorf("could not read the process tree of codex: %w", err)
	}

	type candidate struct {
		thread string
		at     time.Time
	}
	var candidates []candidate
	seen := map[string]bool{}

	for _, pid := range processes {
		files, err := proc.OpenFiles(pid)
		if err != nil {
			continue
		}
		for _, file := range files {
			if !strings.HasPrefix(file, prefix) || !strings.HasSuffix(file, ".lock") {
				continue
			}
			thread := strings.TrimSuffix(filepath.Base(file), ".lock")
			if thread == "" || seen[thread] {
				continue
			}
			seen[thread] = true

			info, err := os.Stat(file)
			if err != nil {
				continue
			}
			candidates = append(candidates, candidate{thread: thread, at: info.ModTime()})
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("codex has not opened a thread yet")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].at.After(candidates[j].at) })
	return candidates[0].thread, nil
}

// Home is where Codex keeps its state, and where delivery has to look.
func Home() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".codex"
	}
	return filepath.Join(home, ".codex")
}

// introValue builds the developer instructions for one launch: the user's own
// value, then the briefing. The key replaces rather than appends, so passing the
// briefing alone would silently drop whatever the user had configured.
func introValue(home, name string) (string, error) {
	existing, err := configString(home, introKey)
	if err != nil {
		return "", err
	}
	intro := harness.Intro(name)
	if strings.TrimSpace(existing) == "" {
		return intro, nil
	}
	return existing + "\n\n" + intro, nil
}

// writableRoots reports the roots to pass when the state directory would
// otherwise be out of the sandbox's reach.
func writableRoots(home, dir string) ([]string, bool) {
	if !configBool(home, "exclude_slash_tmp") {
		return nil, false
	}
	roots := configArray(home, "writable_roots")
	for _, root := range roots {
		if root == dir {
			return nil, false
		}
	}
	return append(roots, dir), true
}

// hasConfigKey reports whether the caller already set a configuration key.
func hasConfigKey(args []string, key string) bool {
	for index, arg := range args {
		if arg == configFlag && index+1 < len(args) && strings.HasPrefix(args[index+1], key+"=") {
			return true
		}
	}
	return false
}

// quoteTOML renders a string as a TOML basic string, which -c parses as a value.
func quoteTOML(value string) string { return strconv.Quote(value) }

func quoteTOMLArray(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, quoteTOML(value))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
