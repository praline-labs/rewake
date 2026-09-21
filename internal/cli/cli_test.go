package cli

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/role"

	// The command table is derived from the harness catalog, so every test
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
	result, err := parse([]string{"--name", "api", "claude", "--model", "chosen-model", "--", "write the notes", "", "--json"})
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
	want := []string{"--model", "chosen-model", "--", "write the notes", "", "--json"}
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

// The guide answers for whoever is asking: a session sees its own role first,
// a shell that is not a session sees the general map unchanged.
func TestGuideShowsTheCallersRole(t *testing.T) {
	play := role.Write.Play
	text := formatGuide(&play)
	if !strings.Contains(text, "YOUR ROLE") || !strings.Contains(text, play.Heading) {
		t.Fatal("a session's guide does not open with its role")
	}
	for _, step := range play.Steps {
		if !strings.Contains(text, step.Do) {
			t.Fatalf("step %q is missing from the guide", step.Do)
		}
	}
	// The same words in the machine form, for an agent that reads --json.
	model := guideModel(&play)
	section, ok := model["role"].(map[string]any)
	if !ok || section["heading"] != play.Heading {
		t.Fatalf("the machine form has no role section: %#v", model["role"])
	}

	plain := formatGuide(nil)
	if strings.Contains(plain, "YOUR ROLE") {
		t.Fatal("a caller with no session was given a role")
	}
	if _, present := guideModel(nil)["role"]; present {
		t.Fatal("the machine form carries a role for a caller that has none")
	}
}
