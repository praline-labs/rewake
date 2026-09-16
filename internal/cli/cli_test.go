package cli

import (
	"bytes"
	"strings"
	"testing"

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
	result, err := parse([]string{"--name", "api", "claude", "--model", "haiku", "--", "--json"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Call.Command == nil || result.Call.Command.Name != "claude" {
		t.Fatalf("command = %v, want claude", result.Call.Command)
	}
	if got := result.Call.Flag("name", ""); got != "api" {
		t.Errorf("name = %q, want api", got)
	}
	want := []string{"--model", "haiku", "--", "--json"}
	if strings.Join(result.Call.Raw, " ") != strings.Join(want, " ") {
		t.Errorf("raw = %v, want %v", result.Call.Raw, want)
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
	code, out, errOut := run("guide", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, errOut)
	}
	for _, want := range []string{`"groups"`, `"flow"`, `"globalOptions"`, `"harnesses"`, `"claude"`, `"codex"`} {
		if !strings.Contains(out, want) {
			t.Errorf("guide --json is missing %q", want)
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

func TestUnbuiltCommandFailsWithReason(t *testing.T) {
	code, _, errOut := run("send", "api", "text")
	if code != ExitFailed {
		t.Fatalf("exit = %d, want %d", code, ExitFailed)
	}
	if !strings.Contains(errOut, "milestone 3") {
		t.Errorf("stub failure does not say what is missing: %s", errOut)
	}
}
