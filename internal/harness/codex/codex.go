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
	"strings"
	"syscall"
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
	var notes []string

	// Both overrides below replace a value rather than add to one, so each is
	// passed only when what it would replace is fully known. A profile, or a
	// setting the caller passed themselves, is a layer this adapter cannot read
	// — and overriding an unread layer silently discards the user's own
	// configuration.
	layered := hasProfile(args)

	if request.Intro && !hasConfigKey(args, introKey) {
		switch {
		case layered:
			notes = append(notes, "not adding the rewake briefing: a profile is selected and its instructions cannot be read from here")
		default:
			instructions, state, err := introValue(home, request.Name)
			if err != nil {
				return harness.LaunchPlan{}, err
			}
			if state == unreadable {
				notes = append(notes, "not adding the rewake briefing: "+introKey+" in config.toml is in a form rewake does not read, and the flag would replace it")
			} else {
				args = harness.AddFlags(args, configFlag, introKey+"="+quoteTOML(instructions))
			}
		}
	}

	// The state directory is under /tmp, which the sandbox may write to by
	// default. A configuration that excludes /tmp would leave the agent unable
	// to send anything, so the directory joins the writable roots without
	// disturbing the ones already there.
	roots, needed, note := writableRoots(home, request.Dir, args, layered)
	if note != "" {
		notes = append(notes, note)
	}
	if needed {
		args = harness.AddFlags(args, configFlag, rootsKey+"="+quoteTOMLArray(roots))
	}

	return harness.LaunchPlan{
		Command:   "codex",
		Args:      args,
		Env:       harness.SessionEnv(request, nil),
		CodexHome: home,
		Notes:     notes,
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
	if err != nil {
		return classify(output, err)
	}

	// The session can change threads while the message is on its way: /new in
	// the middle leaves the text queued for a conversation nobody is looking at.
	// Queueing it again would put a copy in both, and a repeated instruction is
	// worse than a missing one — so the sender is told instead, and decides.
	if now, err := CurrentThread(session.HarnessPID, home); err == nil && now != thread {
		return inbox.Result{
			State:  inbox.Failed,
			Detail: "the codex session started a new conversation while this was being queued; it went to the previous one. Send it again if it still applies",
		}
	}
	return inbox.Result{State: inbox.Delivered, Via: "codex queue", Detail: pollNotice}
}

// queue is the call to the Codex CLI, replaceable in tests.
//
// The deadline has to bind the wait as well as the process. CombinedOutput waits
// for the pipes to close, and a child that outlives the command keeps them open:
// measured, a call with a 100 ms deadline returned after two seconds because the
// grandchild was still holding them. WaitDelay is what stops that.
var queue = func(ctx context.Context, home, thread, text string) (string, error) {
	command := exec.CommandContext(ctx, "codex", "queue", "--thread", thread, "--message", text)
	command.Env = append(os.Environ(), "CODEX_HOME="+home)
	command.WaitDelay = 2 * time.Second
	// Its own process group, so the deadline reaches whatever it started. The
	// command may be a launcher, and killing only the launcher left a child that
	// went on to queue the message after delivery had been reported failed.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
			return command.Process.Kill()
		}
		return nil
	}

	var output strings.Builder
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	return output.String(), err
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
	// /proc reports resolved paths, so a CODEX_HOME that goes through a symlink
	// never matches the prefix unless it is resolved here too. Without this the
	// lock is found and rejected, and every message sits pending until it expires.
	resolved := home
	if absolute, err := filepath.Abs(resolved); err == nil {
		// /proc reports absolute, resolved paths. A relative CODEX_HOME never
		// matched, and every message sat pending until it expired.
		resolved = absolute
	}
	if link, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = link
	}
	prefix := filepath.Join(resolved, lockDir) + string(filepath.Separator)

	// The search goes outwards one generation at a time and stops at the first
	// one holding a thread. A child may be a Codex run of its own — a tool
	// calling `codex exec` — and its thread belongs to it, not to the session
	// somebody addressed. Taking the newest lock in the whole tree picked that
	// nested run whenever the command was a launcher without a lock of its own.
	generation := []int{harnessPID}
	seen := map[int]bool{harnessPID: true}
	for depth := 0; depth < 8 && len(generation) > 0; depth++ {
		if thread, err := threadOf(generation, prefix); err == nil {
			return thread, nil
		}

		var next []int
		for _, pid := range generation {
			children, err := proc.Children(pid)
			if err != nil {
				continue
			}
			for _, child := range children {
				if !seen[child] {
					seen[child] = true
					next = append(next, child)
				}
			}
		}
		generation = next
	}
	return "", fmt.Errorf("codex has not opened a thread yet")
}

// threadOf picks the most recently touched thread lock held by these processes.
func threadOf(processes []int, prefix string) (string, error) {

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

// rootsKey is the sandbox setting that lists what the agent may write to.
const rootsKey = sandboxSection + ".writable_roots"

// excludeTmpKey says whether /tmp has been taken out of the sandbox's reach.
const excludeTmpKey = "exclude_slash_tmp"

// introValue builds the developer instructions for one launch: the user's own
// value, then the briefing. The key replaces rather than appends, so passing the
// briefing alone would silently drop whatever the user had configured.
func introValue(home, name string) (string, reading, error) {
	existing, state, err := configString(home, introKey)
	if err != nil {
		return "", state, err
	}
	if state == unreadable {
		return "", state, nil
	}
	intro := harness.Intro(name)
	if strings.TrimSpace(existing) == "" {
		return intro, state, nil
	}
	return existing + "\n\n" + intro, state, nil
}

// writableRoots reports the roots to pass when the state directory would
// otherwise be out of the sandbox's reach, and a note when it cannot tell.
func writableRoots(home, dir string, args []string, layered bool) ([]string, bool, string) {
	if hasConfigKey(args, rootsKey) || hasConfigKey(args, sandboxSection+"."+excludeTmpKey) {
		// The caller configured the sandbox themselves; their value is the one
		// they meant, and replacing it would drop paths rewake never saw.
		return nil, false, ""
	}

	excluded, state := configBool(home, excludeTmpKey)
	if state == unreadable {
		return nil, false, "cannot tell whether /tmp is writable in the sandbox: " + excludeTmpKey + " is in a form rewake does not read. If the agent cannot send messages, add " + dir + " to " + rootsKey
	}
	if state == missing || !excluded {
		return nil, false, ""
	}
	if layered {
		return nil, false, "a profile is selected, so the sandbox roots cannot be read from here; if the agent cannot send messages, add " + dir + " to " + rootsKey
	}

	roots, rootsState := configArray(home, "writable_roots")
	if rootsState == unreadable {
		return nil, false, "not extending " + rootsKey + ": it is in a form rewake does not read, and the flag would replace it. Add " + dir + " there to let the agent send messages"
	}
	for _, root := range roots {
		if root == dir {
			return nil, false, ""
		}
	}
	return append(roots, dir), true, ""
}

// hasConfigKey reports whether the caller already set a configuration key, in
// either spelling of the flag. Anything after "--" is input for the harness,
// not a flag.
func hasConfigKey(args []string, key string) bool {
	visible := harness.BeforeTerminator(args)
	for index, arg := range visible {
		// Both spellings, and both shapes: "-c key=value" and "--config=key=value"
		// are the same instruction, and missing one of them means overriding a
		// setting the caller had already made.
		switch {
		case arg == configFlag || arg == "--config":
			if index+1 < len(visible) && strings.HasPrefix(visible[index+1], key+"=") {
				return true
			}
		case strings.HasPrefix(arg, "--config="):
			if strings.HasPrefix(strings.TrimPrefix(arg, "--config="), key+"=") {
				return true
			}
		case strings.HasPrefix(arg, "-c") && len(arg) > 2:
			if strings.HasPrefix(arg[2:], key+"=") {
				return true
			}
		}
	}
	return false
}

// hasProfile reports whether the caller selected a configuration profile. Its
// values are a layer this adapter cannot read, so overriding on top of one would
// replace something it never saw.
func hasProfile(args []string) bool {
	for _, arg := range harness.BeforeTerminator(args) {
		// Including the joined short form: "-pwork" selects a profile as surely
		// as "-p work" does.
		if arg == "-p" || arg == "--profile" || strings.HasPrefix(arg, "--profile=") ||
			(strings.HasPrefix(arg, "-p") && len(arg) > 2) {
			return true
		}
	}
	return false
}

// quoteTOML renders a string as a TOML basic string.
//
// Not strconv.Quote: Go escapes a control character as \xNN, which TOML does not
// define. Codex then fails to parse the value and falls back to taking the
// argument as a raw string, so the agent receives quotes and escapes instead of
// its instructions.
func quoteTOML(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, symbol := range value {
		switch symbol {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if symbol < 0x20 || symbol == 0x7f {
				out.WriteString(fmt.Sprintf(`\u%04X`, symbol))
				continue
			}
			out.WriteRune(symbol)
		}
	}
	out.WriteByte('"')
	return out.String()
}

func quoteTOMLArray(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, quoteTOML(value))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
