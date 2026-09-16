package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

func launchWith(t *testing.T, config string, args []string) harness.LaunchPlan {
	t.Helper()
	codexHome(t, config)
	plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: t.TempDir(), Intro: true, Args: args})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	return plan
}

// The override replaces the user's value, so a value this reader does not
// understand must stop it. Every one of these is valid TOML that the reader
// cannot take apart.
func TestUnreadableInstructionsAreNotOverridden(t *testing.T) {
	cases := map[string]string{
		"trailing comment": "developer_instructions = \"Keep my rules\" # required\n",
		"literal string":   "developer_instructions = 'Keep my rules'\n",
		"unclosed block":   "developer_instructions = \"\"\"\nKeep my rules\n",
	}

	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			plan := launchWith(t, config, nil)
			value, passed := configValue(plan.Args, introKey)

			if !passed {
				if len(plan.Notes) == 0 {
					t.Fatal("the briefing was skipped without saying so")
				}
				return
			}
			if !strings.Contains(value, "Keep my rules") {
				t.Fatalf("the user's instructions were replaced by %s", value)
			}
		})
	}
}

// Same for the sandbox: replacing a list that could not be read whole would drop
// the paths that were not seen.
func TestUnreadableSandboxRootsAreNotOverridden(t *testing.T) {
	config := "[sandbox_workspace_write]\nexclude_slash_tmp = true\nwritable_roots = [\n  \"/var/data\",\n]\n"
	plan := launchWith(t, config, nil)

	if _, passed := configValue(plan.Args, rootsKey); passed {
		t.Fatalf("a multi-line list was replaced: %v", plan.Args)
	}
	if len(plan.Notes) == 0 {
		t.Fatal("nothing was said about the sandbox setting rewake could not extend")
	}
}

// A comma inside a path is not a separator.
func TestCommaInsideAPathIsKept(t *testing.T) {
	config := "[sandbox_workspace_write]\nexclude_slash_tmp = true\nwritable_roots = [\"/work/a,b\"]\n"
	plan := launchWith(t, config, nil)

	value, passed := configValue(plan.Args, rootsKey)
	if !passed {
		t.Fatalf("the state directory was not added: %v", plan.Args)
	}
	if !strings.Contains(value, "/work/a,b") {
		t.Errorf("roots = %s, want the configured path intact", value)
	}
}

func TestCommentAfterABooleanIsNotPartOfIt(t *testing.T) {
	config := "[sandbox_workspace_write]\nexclude_slash_tmp = true # required\n"
	plan := launchWith(t, config, nil)

	if _, passed := configValue(plan.Args, rootsKey); !passed {
		t.Fatalf("the state directory was not made writable: %v %v", plan.Args, plan.Notes)
	}
}

// A profile and a caller's own override are layers this adapter cannot read, and
// replacing what it has not read discards the user's configuration.
func TestLayeredConfigurationIsLeftAlone(t *testing.T) {
	for _, args := range [][]string{
		{"-p", "work"},
		{"--profile=work"},
		{"--config", introKey + `="mine"`},
	} {
		plan := launchWith(t, "", args)
		if value, passed := configValue(plan.Args, introKey); passed && !strings.Contains(value, "mine") {
			t.Errorf("%v: the briefing overrode a layer rewake cannot read: %s", args, value)
		}
	}
}

func TestCallerSandboxOverrideIsLeftAlone(t *testing.T) {
	config := "[sandbox_workspace_write]\nexclude_slash_tmp = true\n"
	plan := launchWith(t, config, []string{"-c", rootsKey + `=["/cli/work"]`})

	count := 0
	for index, arg := range plan.Args {
		if arg == "-c" && index+1 < len(plan.Args) && strings.HasPrefix(plan.Args[index+1], rootsKey+"=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("the sandbox roots were set %d times, want the caller's only: %v", count, plan.Args)
	}
}

// Go's quoting is not TOML's: \xNN is undefined there, and Codex then falls back
// to reading the argument as a raw string — the agent gets quotes and escapes
// instead of its instructions.
func TestQuotingIsTOMLNotGo(t *testing.T) {
	for _, value := range []string{"line\nline", "tab\there", "quote\"inside", "escape\x1b[0m", "back\\slash"} {
		quoted := quoteTOML(value)
		if strings.Contains(quoted, `\x`) {
			t.Errorf("%q was quoted with a Go escape TOML does not define: %s", value, quoted)
		}
		decoded, ok := basicString(quoted)
		if !ok {
			t.Errorf("%q produced something this reader cannot read back: %s", value, quoted)
			continue
		}
		if decoded != value {
			t.Errorf("round trip changed %q into %q", value, decoded)
		}
	}
}

func TestArrayQuotingIsAnArray(t *testing.T) {
	quoted := quoteTOMLArray([]string{"/one", "/two"})
	if !strings.HasPrefix(quoted, "[") || !strings.HasSuffix(quoted, "]") {
		t.Fatalf("roots were not rendered as an array: %s", quoted)
	}
	if strings.Count(quoted, `"`) != 4 {
		t.Errorf("array = %s, want two quoted items", quoted)
	}
}

// /proc reports resolved paths. A CODEX_HOME that goes through a symlink would
// otherwise never match, and every message would sit pending until it expired.
func TestThreadIsFoundThroughASymlinkedHome(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	pid := threadFixture(t, real, map[string]time.Time{"01a0-thread": time.Now()})

	thread, err := CurrentThread(pid, link)
	if err != nil {
		t.Fatalf("CurrentThread through a symlink: %v", err)
	}
	if thread != "01a0-thread" {
		t.Errorf("thread = %q, want the one the process holds", thread)
	}
}

// A tool inside the session can run Codex of its own. That nested run has its own
// thread, and it is not the one somebody addressed by the session's name.
func TestNestedCodexThreadIsNotTaken(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	locks := filepath.Join(home, lockDir)
	if err := os.MkdirAll(locks, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// The session holds an older lock; its child holds a newer one.
	writeProcess(t, root, 7, 1, filepath.Join(locks, "session.lock"), time.Now().Add(-time.Hour))
	writeProcess(t, root, 8, 7, filepath.Join(locks, "nested.lock"), time.Now())
	swapProcRoot(t, root)

	thread, err := CurrentThread(7, home)
	if err != nil {
		t.Fatalf("CurrentThread: %v", err)
	}
	if thread != "session" {
		t.Errorf("thread = %q, want the session's own even though the nested one is newer", thread)
	}
}

// TOML lets a key be quoted, and "developer_instructions" is the same key as the
// bare one. Not recognising that meant the value was never found — and then
// replaced by the briefing.
func TestQuotedKeyIsTheSameKey(t *testing.T) {
	plan := launchWith(t, "\"developer_instructions\" = \"Keep my rules\"\n", nil)

	value, passed := configValue(plan.Args, introKey)
	if passed && !strings.Contains(value, "Keep my rules") {
		t.Fatalf("a quoted key was overridden: %s", value)
	}
}

// A key that comes after other settings has to be found too: stopping at the
// first line that is not it would lose the value and then replace it.
func TestKeyIsFoundAfterOtherSettings(t *testing.T) {
	config := "model = \"gpt-5.6-terra\"\napproval_policy = \"never\"\ndeveloper_instructions = \"Keep my rules\"\n"
	plan := launchWith(t, config, nil)

	value, passed := configValue(plan.Args, introKey)
	if !passed {
		t.Fatalf("the briefing was skipped: %v", plan.Notes)
	}
	if !strings.Contains(value, "Keep my rules") {
		t.Errorf("instructions further down the file were dropped: %s", value)
	}
}

// A multi-line literal is not a finished single-line one. Taking it for one
// turned the user's instructions into a lone apostrophe.
func TestMultilineLiteralIsNotMistakenForAShortOne(t *testing.T) {
	plan := launchWith(t, "developer_instructions = '''\nKeep my rules\n'''\n", nil)

	if value, passed := configValue(plan.Args, introKey); passed {
		t.Fatalf("a multi-line literal was replaced by %s", value)
	}
	if len(plan.Notes) == 0 {
		t.Error("the briefing was skipped without saying so")
	}
}

// The escapes of a multi-line basic string have to be resolved: a value holding
// \n otherwise reaches the harness as those two characters, quoted correctly and
// wrong.
func TestMultilineEscapesAreDecoded(t *testing.T) {
	plan := launchWith(t, "developer_instructions = \"\"\"\nfirst\\nsecond\n\"\"\"\n", nil)

	value, passed := configValue(plan.Args, introKey)
	if !passed {
		t.Fatalf("the briefing was skipped: %v", plan.Notes)
	}
	decoded, ok := basicString(value)
	if !ok {
		t.Fatalf("the override is not a readable TOML string: %s", value)
	}
	if strings.Contains(decoded, `\n`) {
		t.Errorf("an escape survived as text: %q", decoded)
	}
	if !strings.Contains(decoded, "first\nsecond") {
		t.Errorf("value = %q, want the escape resolved into a line break", decoded)
	}
}

// Both spellings and both shapes of the flags Codex accepts count as the
// caller's own setting; missing one means overriding what they configured.
func TestEveryFormOfACallerOverrideIsSeen(t *testing.T) {
	for _, args := range [][]string{
		{"--config=" + introKey + `="mine"`},
		{"-c" + introKey + `="mine"`},
		{"-pwork"},
		{"-p", "work"},
	} {
		plan := launchWith(t, "", args)
		if value, passed := configValue(plan.Args, introKey); passed && !strings.Contains(value, "mine") {
			t.Errorf("%v: rewake overrode a setting the caller had made: %s", args, value)
		}
	}
}

// What the caller passed has to survive: dropping their arguments while
// declining to add ours would be the same loss by another route.
func TestCallerArgumentsSurviveALaunch(t *testing.T) {
	args := []string{"--model", "gpt-5.6-terra", "-pwork", "--", "write the notes"}
	plan := launchWith(t, "", args)

	joined := strings.Join(plan.Args, " ")
	for _, want := range args {
		if !strings.Contains(joined, want) {
			t.Errorf("%q was dropped from the launch: %v", want, plan.Args)
		}
	}
}

// A section header and a setting written inside somebody's instructions are
// text, not configuration. Reading them as configuration let an example in the
// instructions decide the sandbox permissions of the run.
func TestSettingsInsideInstructionsAreText(t *testing.T) {
	config := "developer_instructions = \"\"\"\n" +
		"For example, a sandbox section looks like this:\n" +
		"[sandbox_workspace_write]\n" +
		"writable_roots = [\"/example\"]\n" +
		"\"\"\"\n" +
		"[sandbox_workspace_write]\n" +
		"exclude_slash_tmp = true\n" +
		"writable_roots = [\"/actual\"]\n"
	plan := launchWith(t, config, nil)

	value, passed := configValue(plan.Args, rootsKey)
	if !passed {
		t.Fatalf("the state directory was not made writable: %v %v", plan.Args, plan.Notes)
	}
	if strings.Contains(value, "/example") {
		t.Errorf("a path from the instructions became a sandbox permission: %s", value)
	}
	if !strings.Contains(value, "/actual") {
		t.Errorf("the configured roots were lost: %s", value)
	}
}

// The same on the other side: a key inside a multi-line value is not a key.
func TestKeyInsideInstructionsIsNotAKey(t *testing.T) {
	config := "developer_instructions = '''\nmodel = \"pretend\"\n'''\n" +
		"[sandbox_workspace_write]\nexclude_slash_tmp = true\nwritable_roots = [\"/actual\"]\n"
	plan := launchWith(t, config, nil)

	value, passed := configValue(plan.Args, rootsKey)
	if !passed || !strings.Contains(value, "/actual") {
		t.Errorf("the real configuration was not read past the instructions: %v %v", plan.Args, plan.Notes)
	}
}

// A triple-quoted string that closes on its own line has its escapes resolved
// too: skipping that left \n reaching the harness as two characters.
func TestInlineMultilineEscapesAreDecoded(t *testing.T) {
	plan := launchWith(t, "developer_instructions = \"\"\"first\\nsecond\"\"\"\n", nil)

	value, passed := configValue(plan.Args, introKey)
	if !passed {
		t.Fatalf("the briefing was skipped: %v", plan.Notes)
	}
	decoded, ok := basicString(value)
	if !ok {
		t.Fatalf("the override is not a readable TOML string: %s", value)
	}
	if strings.Contains(decoded, `\n`) {
		t.Errorf("an escape survived as text: %q", decoded)
	}
}

func TestJoinedShortConfigFormIsSeen(t *testing.T) {
	plan := launchWith(t, "", []string{"-c=" + introKey + `="mine"`})

	if value, passed := configValue(plan.Args, introKey); passed && !strings.Contains(value, "mine") {
		t.Errorf("rewake overrode a setting the caller had made: %s", value)
	}
}
