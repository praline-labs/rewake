package claude

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// The name check's argument space (docs/mail-bridge-launch.md#claude-code):
// every launch line built from what a caller may pass, judged by an oracle
// written apart from the check, against a fake claude that answers mcp get as
// 2.1.284 did and records that it ran.

// sentinel stands for a secret in every value, file and answer a check may
// meet; no diagnostic may carry it.
const sentinel = "S3CR3T-sentinel-value"

// part is one thing a caller may pass: its words before "--", its words
// after, what it leaves the run, and, for --mcp-config, the values it adds
// in order.
type part struct {
	label      string
	head, tail []string
	// stops is the mark a no-tool reason carries, "" for a part that
	// leaves the tool in.
	stops  string
	values []string
}

// The --mcp-config values' kinds, as the oracle reads them.
const (
	clean      = "clean"
	named      = "named"
	unreadable = "unreadable"
)

// checkFiles writes the files the space names, in a launch directory whose
// path carries no sentinel, and returns it.
func checkFiles(t *testing.T) string {
	t.Helper()
	// No managed MCP file: the machine's own policy decides nothing here.
	previous := managedDir
	managedDir = filepath.Join(t.TempDir(), "managed")
	t.Cleanup(func() { managedDir = previous })
	cwd := filepath.Join(t.TempDir(), "launch")
	files := map[string]string{
		"settings-" + sentinel + ".json":  `{"env":{"TOKEN":"` + sentinel + `"}}`,
		"clean-" + sentinel + ".json":     `{"mcpServers":{"other":{"command":"x","env":{"T":"` + sentinel + `"}}}}`,
		"named-" + sentinel + ".json":     `{"mcpServers":{"rewake":{"command":"` + sentinel + `"}}}`,
		"broken-" + sentinel + ".json":    `{"mcpServers": ` + sentinel,
		"settings-hooks-" + sentinel:      `{"hooks":"` + sentinel + `"}`,
		"settings-matchers-" + sentinel:   `{"hooks":{"PreToolUse":{"x":"` + sentinel + `"}}}`,
		"settings-null-" + sentinel:       `null`,
		"settings-array-" + sentinel:      `["` + sentinel + `"]`,
		"settings-own-hooks-" + sentinel:  `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"` + sentinel + `"}]}]}}`,
		"clean-servers-null-" + sentinel:  `{"mcpServers":null}`,
		"named-other-case-" + sentinel:    `{"mcpServers":{"Rewake":{"command":"` + sentinel + `"}}}`,
		"clean-no-servers-" + sentinel:    `{"env":"` + sentinel + `"}`,
		"broken-not-object-" + sentinel:   `["` + sentinel + `"]`,
		"named-among-others-" + sentinel:  `{"mcpServers":{"a":{},"rewake":{"env":{"K":"` + sentinel + `"}},"b":{}}}`,
		"settings-status-bad-" + sentinel: `{"statusLine":"` + sentinel + `"}`,
	}
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(cwd, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return cwd
}

// worktreeParts are the spellings of Claude Code's own -w and their near
// misses: -w counts in every form its parser takes, and nowhere after "--".
func worktreeParts() []part {
	return []part{
		{label: "no -w"},
		{label: "-w", head: []string{"-w"}, stops: "-w"},
		{label: "-w name", head: []string{"-w", "fix"}, stops: "-w"},
		{label: "-w=name", head: []string{"-w=fix"}, stops: "-w"},
		{label: "-wname", head: []string{"-wfix"}, stops: "-w"},
		{label: "group -cw", head: []string{"-cw"}, stops: "-w"},
		{label: "-c alone", head: []string{"-c"}},
		{label: "--worktree-like long flag", head: []string{"--model", "x"}},
		{label: "-w after --", tail: []string{"-w"}},
	}
}

func bareParts() []part {
	return []part{
		{label: "no --bare"},
		{label: "--bare", head: []string{"--bare"}, stops: "--bare"},
		{label: "--bare after --", tail: []string{"--bare"}},
	}
}

// settingsParts are the caller's --settings: the last one is what the
// harness reads, and one that cannot be merged leaves the tool out.
func settingsParts(cwd string) []part {
	file := "settings-" + sentinel + ".json"
	return []part{
		{label: "no --settings"},
		{label: "file", head: []string{"--settings", file}},
		{label: "absolute file", head: []string{"--settings", filepath.Join(cwd, file)}},
		{label: "=form file", head: []string{"--settings=" + file}},
		{label: "inline", head: []string{"--settings", `{"env":{"K":"` + sentinel + `"}}`}},
		{label: "own hooks", head: []string{"--settings", "settings-own-hooks-" + sentinel}},
		{label: "missing file", head: []string{"--settings", "missing-" + sentinel + ".json"}, stops: "--settings"},
		{label: "broken inline", head: []string{"--settings", `{"env": ` + sentinel}, stops: "--settings"},
		{label: "hooks not an object", head: []string{"--settings", "settings-hooks-" + sentinel}, stops: "--settings"},
		{label: "matchers not a list", head: []string{"--settings", "settings-matchers-" + sentinel}, stops: "--settings"},
		{label: "null", head: []string{"--settings", "settings-null-" + sentinel}, stops: "--settings"},
		{label: "array", head: []string{"--settings", "settings-array-" + sentinel}, stops: "--settings"},
		{label: "status line not an object", head: []string{"--settings", "settings-status-bad-" + sentinel}, stops: "--settings"},
		{label: "broken then good", head: []string{"--settings", "missing-" + sentinel, "--settings", file}},
		{label: "good then broken", head: []string{"--settings", file, "--settings", "missing-" + sentinel}, stops: "--settings"},
		{label: "broken after --", tail: []string{"--settings", "missing-" + sentinel}},
	}
}

// mcpParts are the caller's --mcp-config: a list per flag, the =form,
// repeated flags, files and inline JSON, and nothing after "--".
func mcpParts(cwd string) []part {
	cleanFile, namedFile := "clean-"+sentinel+".json", "named-"+sentinel+".json"
	cleanInline := `{"mcpServers":{"x":{"command":"` + sentinel + `"}}}`
	namedInline := `{"mcpServers":{"rewake":{"command":"` + sentinel + `"}}}`
	return []part{
		{label: "no --mcp-config"},
		{label: "clean file", head: []string{"--mcp-config", cleanFile}, values: []string{clean}},
		{label: "clean absolute file", head: []string{"--mcp-config", filepath.Join(cwd, cleanFile)}, values: []string{clean}},
		{label: "clean inline", head: []string{"--mcp-config", cleanInline}, values: []string{clean}},
		{label: "servers null", head: []string{"--mcp-config", "clean-servers-null-" + sentinel}, values: []string{clean}},
		{label: "no servers key", head: []string{"--mcp-config", "clean-no-servers-" + sentinel}, values: []string{clean}},
		{label: "other case", head: []string{"--mcp-config", "named-other-case-" + sentinel}, values: []string{clean}},
		{label: "named file", head: []string{"--mcp-config", namedFile}, values: []string{named}},
		{label: "named inline", head: []string{"--mcp-config", namedInline}, values: []string{named}},
		{label: "named among others", head: []string{"--mcp-config", "named-among-others-" + sentinel}, values: []string{named}},
		{label: "list clean named", head: []string{"--mcp-config", cleanFile, namedInline}, values: []string{clean, named}},
		{label: "list clean unreadable named", head: []string{"--mcp-config", cleanInline, "missing-" + sentinel, namedFile}, values: []string{clean, unreadable, named}},
		{label: "=form named", head: []string{"--mcp-config=" + namedFile}, values: []string{named}},
		{label: "two flags", head: []string{"--mcp-config", cleanFile, "--mcp-config", namedInline}, values: []string{clean, named}},
		{label: "=form then list", head: []string{"--mcp-config=" + cleanInline, "--mcp-config", cleanFile, cleanFile}, values: []string{clean, clean, clean}},
		{label: "missing file", head: []string{"--mcp-config", "missing-" + sentinel + ".json"}, values: []string{unreadable}},
		{label: "broken file", head: []string{"--mcp-config", "broken-" + sentinel + ".json"}, values: []string{unreadable}},
		{label: "not an object", head: []string{"--mcp-config", "broken-not-object-" + sentinel}, values: []string{unreadable}},
		{label: "broken inline", head: []string{"--mcp-config", `{"mcpServers": ` + sentinel}, values: []string{unreadable}},
		{label: "named after --", tail: []string{"--mcp-config", namedInline}},
	}
}

// verdict is what the oracle expects of one launch line.
type verdict struct {
	stops []string // any of these marks may be the reason
	// taken or failed name the --mcp-config value, 1-based, that refuses.
	taken, failed int
	asked         bool // mcp get runs
}

// judge reads a line's parts as the rules state them, not as the check
// orders its steps: any part that leaves the tool out may be the reason.
func judge(parts []part, g7 g7Case) verdict {
	var result verdict
	for _, p := range parts {
		if p.stops != "" {
			result.stops = append(result.stops, p.stops)
		}
	}
	if len(result.stops) > 0 {
		return result
	}
	if g7.open != "" {
		result.stops = []string{g7.open}
		return result
	}
	position := 0
	for _, p := range parts {
		for _, value := range p.values {
			position++
			switch value {
			case named:
				result.taken = position
				return result
			case unreadable:
				result.failed = position
				return result
			}
		}
	}
	result.asked = true
	return result
}

// fakeClaude writes a program answering mcp get by a script, which first
// records its directory, its CLAUDE_CONFIG_DIR and its arguments in mark.
func fakeClaude(t *testing.T, dir, mark, body string) string {
	t.Helper()
	path := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nprintf '%s|%s|%s\\n' \"$PWD\" \"$CLAUDE_CONFIG_DIR\" \"$*\" >> '" + mark + "'\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

const absentAnswer = `echo 'No MCP server named "rewake".' >&2; exit 1`

func TestTheNameCheckOverEveryLaunchLine(t *testing.T) {
	cwd := checkFiles(t)
	config := filepath.Join(t.TempDir(), "config")
	mark := filepath.Join(t.TempDir(), "mark")
	program := fakeClaude(t, t.TempDir(), mark, absentAnswer)
	env := []string{"PATH=" + os.Getenv("PATH"), "CLAUDE_CONFIG_DIR=" + config}

	cases, asked := 0, 0
	for _, w := range worktreeParts() {
		for _, b := range bareParts() {
			for _, s := range settingsParts(cwd) {
				for _, m := range mcpParts(cwd) {
					for _, mcpFirst := range []bool{false, true} {
						parts := []part{w, b, s, m}
						if mcpFirst {
							parts = []part{m, w, b, s}
						}
						for _, g7 := range g7Cases() {
							cases++
							want := judge(parts, g7)
							if want.asked {
								asked++
							}
							line := lineOf(parts)
							_ = os.Remove(mark)
							version := g7.version
							decision, err := claudeHarness{}.CheckMailTool(harness.ToolCheckRequest{
								Args: line, Command: program, Cwd: cwd, Env: env, Version: &version, Assumed: g7.assumed,
							})
							if problem := against(want, decision, err, mark); problem != "" {
								t.Fatalf("%q, %s: %s", line, g7.name, problem)
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d launch lines, mcp get asked in %d", cases, asked)
	if asked == 0 || asked == cases {
		t.Fatalf("the space never or always reached mcp get: %d of %d", asked, cases)
	}
}

// g7Case is one way a launch takes G7: the version the launch read, the
// gates assumed, and the reason the rules give when it stays open.
type g7Case struct {
	name    string
	version harness.Version
	assumed []string
	open    string
}

// g7Cases are G7 closed by the table for the version read, assumed, open
// for a version the table does not name, and open for an unknown one
// (docs/mail-bridge-version.md).
func g7Cases() []g7Case {
	return []g7Case{
		{"closed for 2.1.284", harness.Version{Value: "2.1.284"}, nil, ""},
		{"assumed", harness.Version{Unknown: harness.VersionNotInPath}, []string{harness.GateG7}, ""},
		{"open for 2.1.200", harness.Version{Value: "2.1.200"}, nil, "gate G7"},
		{"unknown", harness.Version{Unknown: harness.VersionNotOnPath}, nil, "harness version unknown (no claude on PATH)"},
	}
}

// lineOf joins the parts' words, the tails after one "--".
func lineOf(parts []part) []string {
	var head, tail []string
	for _, p := range parts {
		head = append(head, p.head...)
		tail = append(tail, p.tail...)
	}
	if len(tail) == 0 {
		return head
	}
	return append(append(head, "--"), tail...)
}

// against compares one answer with the oracle's verdict; "" when they agree.
func against(want verdict, decision harness.ToolDecision, err error, mark string) string {
	if err != nil && strings.Contains(err.Error(), sentinel) {
		return "the diagnostic carries a value: " + err.Error()
	}
	if strings.Contains(decision.Reason, sentinel) {
		return "the reason carries a value: " + decision.Reason
	}
	_, statErr := os.Stat(mark)
	if ran := statErr == nil; ran != want.asked {
		return "mcp get ran: " + map[bool]string{true: "yes", false: "no"}[ran]
	}
	var taken *harness.NameTakenError
	var failed *harness.CheckFailedError
	switch {
	case len(want.stops) > 0:
		if err != nil || decision.Inject {
			return "want no tool, got inject " + boolWord(decision.Inject) + " err " + errWord(err)
		}
		for _, stop := range want.stops {
			if strings.Contains(decision.Reason, stop) {
				return ""
			}
		}
		return "the reason " + decision.Reason + " names none of " + strings.Join(want.stops, ", ")
	case want.taken > 0:
		if !errors.As(err, &taken) || taken.Where.Flag != "--mcp-config value" || taken.Where.Position != want.taken || taken.Started {
			return "want the name taken at value " + strconv.Itoa(want.taken) + ", got " + errWord(err)
		}
	case want.failed > 0:
		if !errors.As(err, &failed) || failed.Where == nil || failed.Where.Position != want.failed || failed.Outcome != harness.OutcomeUnreadable {
			return "want value " + strconv.Itoa(want.failed) + " unreadable, got " + errWord(err)
		}
	default:
		if err != nil || !decision.Inject {
			return "want the tool, got " + errWord(err) + " " + decision.Reason
		}
	}
	return ""
}

func boolWord(b bool) string { return map[bool]string{true: "true", false: "false"}[b] }

func errWord(err error) string {
	if err == nil {
		return "no error"
	}
	return err.Error()
}
