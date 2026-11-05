package adapter

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The purity of the agents' halves (brief 94): what the Статус shows, what the
// face is told, a tool's description and a control reply's words are the
// wrapper's, so a change of them reaches a running room by a restart of
// rewake alone. The agents run natively, check just before a call, render
// and carry; each adapter's own test pins its wire (the mod's schema, pi's
// frame), and this one scans the files the agents ship. It is a smoke test,
// not an analyzer: a word renamed slips past it.

// agent is one harness's shipped half: the files the adapter embeds, the
// modules it may import beside its own, the one call that registers a tool,
// and the rules it alone keeps.
type agent struct {
	name     string
	files    map[string]string
	external []string
	register *regexp.Regexp
	// described is the value each file may give a description key: the
	// table's entry where a tool is registered, a relayed option's.
	described map[string]string
	// mirrored says the agent forwards its harness's events raw, so it
	// names none of the mirror's kinds nor the face's tool vocabulary; the
	// mod's mirror still translates (docs/face.md, "The feed").
	mirrored bool
}

// worded are a reply's literal words the scan lets stand, each with why.
var worded = map[string]string{
	`return { decision: "allow", reason: "allowed through rewake: " + answer.toLowerCase() };`: "a permission decision Claude Code hands its model, the asks' own wording, which brief 94 does not move",
	`return { decision: "deny", reason: "Not allowed; the answer was: " + answer };`:           "the same decision, denied",
	`return { decision: "allow", reason: "allowed for this session through rewake" };`:         "the same decision, for the session",
}

var (
	// A key of what the Статус shows, or of a harness's reading it projects.
	statusKey = regexp.MustCompile(`\b(cacheRead|cacheWrite|cacheCreation\w*|cache_read\w*|cache_creation\w*|rateLimits|rate_limits|resetsAt|resets_at|usedPercent\w*|percent|ttl|expiresAt|modelName|contextWindow|tokensLeft)\b`)
	// The face's names of a call and their fields, which pi's translator gives.
	toolKin = regexp.MustCompile(`\b(file_path|old_string|new_string|head_limit)\b|"(Bash|Read|Write|Edit|MultiEdit|Grep|Glob)"`)
	// A description or a reply's words given as a literal: a key, not a
	// field read before a ternary's colon.
	literalDescription = regexp.MustCompile(`(?:^|[^.\w$])description\s*:\s*["'` + "`" + `]`)
	literalReply       = regexp.MustCompile(`(?:^|[^.\w$])(reason|detail)\s*:\s*["'` + "`" + `][^"'` + "`" + `]`)
	descriptionKey     = regexp.MustCompile(`(?:^|[^.\w$])description\s*\??\s*:\s*([^,;}\n]+)`)
	imported           = regexp.MustCompile(`(?:\bfrom|\bimport|\brequire\s*\()\s*\(?\s*"([^"]+)"`)
	eventKind          = regexp.MustCompile(`EventKind = "(\w+)"`)
)

// shipped is what an embed line of the adapter's names, read from dir.
func shipped(t *testing.T, source, dir string) map[string]string {
	t.Helper()
	text, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for line := range strings.SplitSeq(string(text), "\n") {
		names, ok := strings.CutPrefix(line, "//go:embed ")
		if !ok {
			continue
		}
		for name := range strings.FieldsSeq(names) {
			if !strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".gen.ts") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			files[name] = string(body)
		}
	}
	if len(files) == 0 {
		t.Fatalf("%s embeds no TypeScript", source)
	}
	return files
}

func agents(t *testing.T) []agent {
	return []agent{
		{
			name: "the mod", files: shipped(t, "claude/mod.go", "claude"), external: []string{"claude-code", "../protocol.gen.ts"},
			register:  regexp.MustCompile(`\$\.tool\.register\(`),
			described: map[string]string{"mod/hooks/tools.ts": "tool.description", "mod/hooks/asks.ts": "option.description"},
		},
		{
			name: "pi's extension", files: shipped(t, "pi/run.go", "pi"), external: []string{"@earendil-works/pi-coding-agent", "@earendil-works/pi-tui", "node:crypto", "node:net"},
			register:  regexp.MustCompile(`\.registerTool\(`),
			described: map[string]string{"tools.ts": "entry.description"},
			mirrored:  true,
		},
	}
}

// mirrorKinds are the model's event kinds, as core/mirror names them.
func mirrorKinds(t *testing.T) *regexp.Regexp {
	t.Helper()
	text, err := os.ReadFile("../core/mirror/event.go")
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, m := range eventKind.FindAllStringSubmatch(string(text), -1) {
		kinds = append(kinds, m[1])
	}
	if len(kinds) < 10 {
		t.Fatalf("the mirror's kinds: %v", kinds)
	}
	return regexp.MustCompile(`\bkind\s*:\s*"(` + strings.Join(kinds, "|") + `)"`)
}

// code is a line less its comment: a // at its start or after a space.
func code(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
		return ""
	}
	if at := strings.Index(line, " // "); at >= 0 {
		return line[:at]
	}
	return line
}

// violations are what an agent's files say that is the wrapper's, each
// named by its file and line.
func violations(a agent, kinds *regexp.Regexp) []string {
	var out []string
	say := func(file string, n int, format string, args ...any) {
		out = append(out, fmt.Sprintf("%s %s:%d: ", a.name, file, n)+fmt.Sprintf(format, args...))
	}
	own := map[string]bool{}
	for name := range a.files {
		own["./"+filepath.Base(name)] = true
	}
	registered := 0
	for _, file := range slices.Sorted(maps.Keys(a.files)) {
		for i, line := range strings.Split(a.files[file], "\n") {
			n, text := i+1, code(line)
			if text == "" {
				continue
			}
			if m := statusKey.FindString(text); m != "" {
				say(file, n, "names the Статус's %q: send the harness's reading raw, and project it in Go (telemetry.go)", m)
			}
			if a.mirrored {
				if m := kinds.FindString(text); m != "" {
					say(file, n, "makes the mirror's %s: forward the harness's event raw, and translate it in Go (translate.go)", m)
				}
				if m := toolKin.FindString(text); m != "" {
					say(file, n, "names the face's tool vocabulary, %s: the translator gives it (translate_tools.go)", m)
				}
			}
			if literalDescription.MatchString(text) {
				say(file, n, "holds a description: the wrapper's table gives it (core/sessiontools)")
			}
			for _, m := range descriptionKey.FindAllStringSubmatch(text, -1) {
				value := strings.TrimSpace(m[1])
				if value != "string" && value != "unknown" && value != a.described[file] {
					say(file, n, "describes with %q, not the wrapper's table", value)
				}
			}
			if literalReply.MatchString(text) && worded[strings.TrimSpace(text)] == "" {
				say(file, n, "words a reply: send its code and the native result, and word it in Go (core/control)")
			}
			for _, m := range imported.FindAllStringSubmatch(text, -1) {
				if !own[m[1]] && !slices.Contains(a.external, m[1]) {
					say(file, n, "imports %s, outside what it ships", m[1])
				}
			}
			registered += len(a.register.FindAllString(text, -1))
		}
	}
	if registered != 1 {
		out = append(out, fmt.Sprintf("%s registers tools at %d sites: one registers the wrapper's table", a.name, registered))
	}
	return out
}

func TestTheAgentsShipNothingTheWrapperDecides(t *testing.T) {
	kinds := mirrorKinds(t)
	for _, a := range agents(t) {
		for _, v := range violations(a, kinds) {
			t.Error(v)
		}
	}
	for line, why := range worded {
		found := false
		for _, a := range agents(t) {
			for _, text := range a.files {
				found = found || strings.Contains(text, line)
			}
		}
		if !found {
			t.Errorf("a reply let stand is no longer shipped, drop it from worded (%s): %s", why, line)
		}
	}
}

// Each concern's slip, seeded into a copy of a real agent file, is caught;
// a comment naming the same word is not.
func TestThePurityScanCatchesWhatItIsFor(t *testing.T) {
	kinds := mirrorKinds(t)
	cases := []struct {
		agent      int
		file, line string
		caught     string
	}{
		{1, "management.ts", `  const cache = { read: usage.cacheRead };`, `names the Статус's "cacheRead"`},
		{0, "mod/hooks/management.ts", `  const left = 100 - limits.five_hour.usedPercentage;`, `names the Статус's "usedPercentage"`},
		{1, "forward.ts", `  stream.push({ kind: "delta", id, text: e.delta });`, `makes the mirror's kind: "delta"`},
		{1, "forward.ts", `  const input = { file_path: args.path };`, `the face's tool vocabulary, file_path`},
		{1, "tools.ts", `const tick = { name: "brief_tick", description: "Tick an item of a brief" };`, "holds a description"},
		{0, "mod/hooks/tools.ts", "  description: `Tick item ${n}`,", "holds a description"},
		{1, "tools.ts", `      description: describe(entry),`, `describes with "describe(entry)"`},
		// A description kept aside and registered at a second site.
		{1, "extension.ts", `  pi.registerTool({ name: "brief_tick", description: entry.description, parameters: kept });`, "registers tools at 2 sites"},
		{0, "mod/hooks/rewake.ts", `  await $.tool.register({ name: "brief_tick", description: kept, inputSchema: {} });`, "registers tools at 2 sites"},
		{0, "mod/hooks/management.ts", `    return { ok: false, detail: "nothing to compact" };`, "words a reply"},
		{1, "extension.ts", `  respond({ outcome: "refused", reason: 'a compaction is running' });`, "words a reply"},
		{1, "extension.ts", `import { cases } from "../../../face/src/call.cases.ts";`, "imports ../../../face/src/call.cases.ts"},
		{0, "mod/hooks/mirror.ts", `import { bounds } from "../../../../core/mirror/feed.ts";`, "imports ../../../../core/mirror/feed.ts"},
	}
	for _, c := range cases {
		a := agents(t)[c.agent]
		if _, ok := a.files[c.file]; !ok {
			t.Fatalf("%s ships no %s", a.name, c.file)
		}
		a.files[c.file] += "\n" + c.line + "\n"
		got := strings.Join(violations(a, kinds), "\n")
		if !strings.Contains(got, c.caught) {
			t.Errorf("%s with %s in %s: %q, want %q", a.name, c.line, c.file, got, c.caught)
		}
		a = agents(t)[c.agent]
		a.files[c.file] += "\n// " + c.line + "\n"
		if got := violations(a, kinds); len(got) != 0 {
			t.Errorf("the same in a comment: %v", got)
		}
	}
}
