package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"

	// The command table is derived from the harness catalogue, so every test
	// here needs it registered.
	_ "github.com/iiiokojiadbi/rewake/internal/harness/catalog"
)

// run executes one invocation and returns code, stdout and stderr.
func run(argv ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := Run(argv, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestNoArgumentsPrintsGuide(t *testing.T) {
	code, out, errOut := run()
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitOK, errOut)
	}
	for _, want := range []string{"RUN A SESSION", "TALK", "FLOW", "HOW THIS TOOL BEHAVES", "rewake <command> --help"} {
		if !strings.Contains(out, want) {
			t.Errorf("guide is missing %q", want)
		}
	}
}

func TestUnknownCommandSuggestsNearest(t *testing.T) {
	code, _, errOut := run("lst")
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, `Did you mean "list"?`) {
		t.Errorf("refusal does not suggest list: %s", errOut)
	}
}

func TestUnknownFlagIsRefusedWithHint(t *testing.T) {
	code, _, errOut := run("list", "--jsno")
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "list does not take --jsno") {
		t.Errorf("refusal does not name the flag: %s", errOut)
	}
	if !strings.Contains(errOut, "full help: rewake list --help") {
		t.Errorf("refusal does not point at full help: %s", errOut)
	}
}

func TestFlagNeedingValueIsRefused(t *testing.T) {
	code, _, errOut := run("send", "api", "text", "--wait")
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "Flag --wait needs a value.") {
		t.Errorf("unexpected refusal: %s", errOut)
	}
}

func TestLoosePositionalsAreRefused(t *testing.T) {
	// The common mistake: text that is not quoted arrives as separate words.
	code, _, errOut := run("send", "api", "pull", "and", "rerun")
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "Quote multiword text as one argument.") {
		t.Errorf("refusal does not explain quoting: %s", errOut)
	}
}

func TestHarnessArgumentsStayRaw(t *testing.T) {
	result, err := parse([]string{"--name", "api", "claude", "--model", "haiku", "--", "write the notes", "", "--json"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Call.Command == nil || result.Call.Command.Name != "claude" {
		t.Fatalf("command = %v, want claude", result.Call.Command)
	}
	if got := result.Call.Flag("name", ""); got != "api" {
		t.Errorf("name = %q, want api", got)
	}
	// Compared element by element: joining them would hide a lost argument
	// boundary and a dropped empty argument, which is how a prompt gets mangled.
	want := []string{"--model", "haiku", "--", "write the notes", "", "--json"}
	if len(result.Call.Raw) != len(want) {
		t.Fatalf("raw = %q, want %q", result.Call.Raw, want)
	}
	for index := range want {
		if result.Call.Raw[index] != want[index] {
			t.Errorf("raw[%d] = %q, want %q", index, result.Call.Raw[index], want[index])
		}
	}
	// A flag meant for the harness must not become a rewake flag.
	if result.Call.Switch("json") {
		t.Error("--json after the harness name was read as a rewake flag")
	}
}

func TestLaunchFlagRejectedOnOtherCommands(t *testing.T) {
	code, _, errOut := run("--name", "api", "list")
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "list does not take --name") {
		t.Errorf("unexpected refusal: %s", errOut)
	}
}

func TestHelpForEveryCommand(t *testing.T) {
	for _, group := range Groups() {
		for _, command := range group.Commands {
			// A launch command passes everything after its name to the harness;
			// --help as the first token is the documented exception.
			code, out, errOut := run(command.Name, "--help")
			if code != ExitOK {
				t.Fatalf("%s --help exit = %d (stderr: %s)", command.Name, code, errOut)
			}
			if !strings.Contains(out, "rewake "+command.Label()) {
				t.Errorf("%s --help does not print its own syntax", command.Name)
			}
			if !strings.Contains(out, "GLOBAL OPTIONS") {
				t.Errorf("%s --help does not print global options", command.Name)
			}
		}
	}
}

func TestTableIsComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, group := range Groups() {
		for _, command := range group.Commands {
			if seen[command.Name] {
				t.Errorf("command %q is listed twice", command.Name)
			}
			seen[command.Name] = true
			if command.Summary == "" {
				t.Errorf("command %q has no summary", command.Name)
			}
			if command.Handler == nil {
				t.Errorf("command %q has no handler", command.Name)
			}
			if len(command.Examples) == 0 {
				t.Errorf("command %q has no examples", command.Name)
			}
		}
	}
	for _, name := range []string{"claude", "codex", "list", "send", "whoami", "guide"} {
		if !seen[name] {
			t.Errorf("command %q is missing from the table", name)
		}
	}

	// Two commands sharing one handler means one of them does the other's work.
	// Launch commands are the exception: they share one closure and differ by
	// the harness they carry, which is checked instead.
	handlers := map[uintptr]string{}
	for _, group := range Groups() {
		for _, command := range group.Commands {
			if command.Harness != nil {
				if command.Harness.ID() != command.Name {
					t.Errorf("command %q starts the harness %q", command.Name, command.Harness.ID())
				}
				continue
			}
			pointer := reflect.ValueOf(command.Handler).Pointer()
			if other, taken := handlers[pointer]; taken {
				t.Errorf("commands %q and %q share a handler", other, command.Name)
			}
			handlers[pointer] = command.Name
		}
	}
}

// Examples are copied verbatim by whoever reads them, so every example must be
// a call this parser accepts, against the command that advertises it.
func TestExamplesParse(t *testing.T) {
	for _, group := range Groups() {
		for _, command := range group.Commands {
			for _, example := range command.Examples {
				fields := splitExample(example)
				if len(fields) == 0 || fields[0] != "rewake" {
					t.Errorf("%s: example %q does not start with rewake", command.Name, example)
					continue
				}
				result, err := parse(fields[1:])
				if err != nil {
					t.Errorf("%s: example %q does not parse: %v", command.Name, example, err)
					continue
				}
				if result.Call.Command == nil || result.Call.Command.Name != command.Name {
					t.Errorf("%s: example %q selects another command", command.Name, example)
					continue
				}
				// The arguments have to survive the parse as well: an example
				// that loses its target teaches a call that does nothing.
				positionals := len(fields) - 2
				for _, field := range fields[2:] {
					if strings.HasPrefix(field, "--") {
						positionals--
					}
				}
				if !command.Raw && positionals > 0 && len(result.Call.Positionals) == 0 {
					t.Errorf("%s: example %q lost its arguments in parsing", command.Name, example)
				}
			}
		}
	}
}

// splitExample splits an example the way a shell would: double quotes hold a
// multiword argument together. Splitting on spaces instead would turn every
// quoted example into the very mistake the parser refuses.
func splitExample(example string) []string {
	var fields []string
	var current strings.Builder
	quoted, started := false, false
	for _, r := range example {
		switch {
		case r == '"':
			quoted = !quoted
			started = true
		case r == ' ' && !quoted:
			if started {
				fields = append(fields, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if started {
		fields = append(fields, current.String())
	}
	return fields
}

func TestGuideJSONCarriesTheTable(t *testing.T) {
	for _, argv := range [][]string{{"guide", "--json"}, {"--json"}} {
		code, out, errOut := run(argv...)
		if code != ExitOK {
			t.Fatalf("%v: exit = %d (stderr: %s)", argv, code, errOut)
		}

		var model struct {
			Groups []struct {
				Commands []struct {
					Name    string `json:"name"`
					Options []struct {
						Flag string `json:"flag"`
					} `json:"options"`
				} `json:"commands"`
			} `json:"groups"`
			Harnesses []string `json:"harnesses"`
		}
		if err := json.Unmarshal([]byte(out), &model); err != nil {
			t.Fatalf("%v: the machine form is not JSON: %v", argv, err)
		}

		flags := map[string]bool{}
		found := false
		for _, group := range model.Groups {
			for _, command := range group.Commands {
				if command.Name != "send" {
					continue
				}
				found = true
				for _, option := range command.Options {
					flags[option.Flag] = true
				}
			}
		}
		if !found {
			t.Errorf("%v: the command table is missing from the machine form", argv)
		}
		if !flags["--wait"] {
			t.Errorf("%v: send is listed without its flags", argv)
		}
		if len(model.Harnesses) == 0 {
			t.Errorf("%v: no harnesses listed", argv)
		}
	}
}

func TestVersion(t *testing.T) {
	code, out, _ := run("--version")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, Version) {
		t.Errorf("version output = %q, want %q", out, Version)
	}
}

// liveSession publishes a session served by nobody, in an isolated state
// directory. This process stands in for the wrapper: it is alive, so the record
// is alive, which is all the sender needs to accept a message.
func liveSession(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Setenv(state.DirEnv, dir)
	// The tests may run inside a rewake session themselves; its name and run
	// must not leak into the sessions they make up.
	t.Setenv(state.SessionEnv, "")
	t.Setenv(state.EpochEnv, "")
	resolved, err := state.Dir()
	if err != nil {
		t.Fatalf("state.Dir: %v", err)
	}

	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatalf("start time: %v", err)
	}
	session := registry.Session{
		Name:         name,
		Harness:      "claude",
		ServicePID:   os.Getpid(),
		ServiceStart: start,
		CWD:          resolved,
		StartedAt:    time.Now(),
		Socket:       filepath.Join(resolved, "sock", name+".sock"),
	}
	if err := registry.Publish(resolved, session); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// A process of the session carries its run, as one started by the wrapper does.
	t.Setenv(state.EpochEnv, session.Epoch())
	return resolved
}

func TestListShowsALiveSession(t *testing.T) {
	liveSession(t, "api")

	code, out, errOut := run("list")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, errOut)
	}
	// The empty answer names a session too, in its hint, so the test asks for
	// the line that only a listed session produces.
	if strings.Contains(out, "No sessions are running") {
		t.Fatalf("list reported nothing running: %q", out)
	}
	first := strings.Fields(strings.Split(out, "\n")[0])
	if len(first) < 2 || first[0] != "api" || first[1] != "claude" {
		t.Errorf("first line = %q, want the session name and its harness", out)
	}
}

func TestSendToUnknownSessionNamesTheLiveOnes(t *testing.T) {
	liveSession(t, "api")

	code, _, errOut := run("send", "web", "hello")
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "No session named \"web\"") {
		t.Errorf("refusal does not name the target: %s", errOut)
	}
	if !strings.Contains(errOut, "Running now: api") {
		t.Errorf("refusal does not say who is reachable: %s", errOut)
	}
}

// Nothing serves the mailbox here, so the message is accepted and stays
// pending: exit code 3, and the message waiting on disk for whoever serves it.
func TestSendWithoutAServerIsPending(t *testing.T) {
	dir := liveSession(t, "api")

	code, out, errOut := run("send", "api", "hello", "--wait", "0.3")
	if code != ExitPending {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitPending, errOut)
	}
	if !strings.Contains(out, "pending for api") {
		t.Errorf("stdout does not explain the pending result: %q", out)
	}

	waiting, err := filepath.Glob(filepath.Join(state.InboxPath(dir, "api"), "*.json"))
	if err != nil || len(waiting) != 1 {
		t.Fatalf("mailbox holds %v, want exactly one message (%v)", waiting, err)
	}
	raw, err := os.ReadFile(waiting[0])
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var written struct {
		To      string `json:"to"`
		From    string `json:"from"`
		Text    string `json:"text"`
		ToEpoch string `json:"toEpoch"`
	}
	if err := json.Unmarshal(raw, &written); err != nil {
		t.Fatalf("the waiting message is not readable: %v", err)
	}
	if written.Text != "hello" || written.To != "api" {
		t.Errorf("message = %+v, want the text and target as sent", written)
	}
	if written.ToEpoch == "" {
		t.Error("the message carries no epoch, so a later session with this name would receive it")
	}
}

func TestSendRefusesEmptyText(t *testing.T) {
	liveSession(t, "api")

	code, _, errOut := run("send", "api", "   ")
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "The message is empty.") {
		t.Errorf("unexpected refusal: %s", errOut)
	}
}

func TestWhoamiOutsideASession(t *testing.T) {
	liveSession(t, "api")
	t.Setenv(state.SessionEnv, "")

	code, out, _ := run("whoami")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "not part of a rewake session") {
		t.Errorf("whoami does not say the shell is unnamed: %q", out)
	}
}
