package codex

import (
	"os"
	"path/filepath"
	"strconv"
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

// Go's quoting is not TOML's: \xNN is undefined there, and Codex then falls back
// to reading the argument as a raw string — the agent gets quotes and escapes
// instead of its instructions.
func TestQuotingIsTOMLNotGo(t *testing.T) {
	for _, value := range []string{"line\nline", "tab\there", "quote\"inside", "escape\x1b[0m", "back\\slash"} {
		quoted := quoteTOML(value)
		if strings.Contains(quoted, `\x`) {
			t.Errorf("%q was quoted with a Go escape TOML does not define: %s", value, quoted)
		}
		decoded, ok := unquoteBasic(quoted)
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
	actualHome := t.TempDir()
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(actualHome, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	pid := threadFixture(t, actualHome, map[string]time.Time{"01a0-thread": time.Now()})

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

func TestJoinedShortConfigFormIsSeen(t *testing.T) {
	plan := launchWith(t, "", []string{"-c=" + introKey + `="mine"`})

	if value, passed := configValue(plan.Args, introKey); passed && !strings.Contains(value, "mine") {
		t.Errorf("rewake overrode a setting the caller had made: %s", value)
	}
}

// unquoteBasic reads back a basic string this adapter wrote.
func unquoteBasic(value string) (string, bool) {
	text, err := strconv.Unquote(value)
	return text, err == nil
}

// The keys rewake would replace are left alone whenever the configuration
// mentions them — in any form, valid TOML or not, including the forms a
// hand-written reader got wrong: prose inside instructions, a string inside an
// array, an escaped quote, a comment after a quote.
func TestAnyMentionKeepsTheBriefingOut(t *testing.T) {
	for name, config := range map[string]string{
		"plain":           "developer_instructions = \"Keep my rules\"\n",
		"after a comment": "other = \"\"\"a \" # b\"\"\"\ndeveloper_instructions = \"Keep these\"\n",
		"inside an array": "other = [\"\"\"\ndeveloper_instructions = 'x'\n\"\"\"]\n",
		"escaped key":     "\"developer_instruction\\u0073\" = \"x\"\n",
		"in a profile":    "[profiles.work]\ndeveloper_instructions = \"x\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			plan := launchWith(t, config, nil)
			if _, passed := configValue(plan.Args, introKey); passed {
				t.Errorf("the briefing replaced configured instructions: %v", plan.Args)
			}
			if len(plan.Notes) == 0 {
				t.Error("the briefing was skipped without saying so")
			}
		})
	}
}

func TestTheBriefingGoesInWhenNothingIsConfigured(t *testing.T) {
	plan := launchWith(t, "model = \"gpt-5\"\n[sandbox_workspace_write]\nwritable_roots = [\"/var/data\"]\n", nil)
	value, passed := configValue(plan.Args, introKey)
	if !passed || !strings.Contains(value, "rewake guide") {
		t.Errorf("briefing = %q (passed %v), want it", value, passed)
	}
	if len(plan.Notes) != 0 {
		t.Errorf("notes = %v, want none for a configuration that does not touch these keys", plan.Notes)
	}
}

// The sandbox roots are never replaced. When /tmp may be excluded, the note says
// what to add, and the roots the user listed stay the only ones.
func TestTheSandboxIsNeverRewritten(t *testing.T) {
	for name, config := range map[string]string{
		"excluded":      "[sandbox_workspace_write]\nexclude_slash_tmp = true\nwritable_roots = [\"/var/data\"]\n",
		"inside prose":  "developer_instructions = \"\"\"\n[sandbox_workspace_write]\nexclude_slash_tmp = true\n\"\"\"\n",
		"default roots": "[sandbox_workspace_write]\nwritable_roots = [\"/var/data\"]\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			codexHome(t, config)
			plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: dir, Intro: false})
			if err != nil {
				t.Fatalf("Launch: %v", err)
			}
			if _, passed := configValue(plan.Args, rootsKey); passed {
				t.Errorf("the sandbox roots were replaced: %v", plan.Args)
			}
			mentions := strings.Contains(config, "exclude_slash_tmp")
			said := strings.Contains(strings.Join(plan.Notes, " "), dir)
			if mentions != said {
				t.Errorf("notes = %v; want the directory to add named exactly when /tmp may be excluded", plan.Notes)
			}
		})
	}
}
