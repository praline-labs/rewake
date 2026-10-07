package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// transition maps each package not yet at its 2.0 path to the layer it is judged
// in and the step that moves it there or removes it
// (docs/v2/stage3-steps.md#s1-the-tests-first, the places from
// docs/v2/stage3-packages.md). An entry fails once its package is at its 2.0
// path or gone, so the table empties with the moves.
var transition = map[string]transit{
	"internal/state":     {layer: infra, step: "S12"},
	"internal/proc":      {layer: infra, step: "S12"},
	"internal/boottime":  {layer: infra, step: "S12"},
	"internal/buildtime": {layer: infra, step: "S12"},

	"internal/receipt":                 {layer: core, step: "S13"},
	"internal/registry":                {layer: core, step: "S13"},
	"internal/registry/registrytest":   {layer: core, step: "S13"},
	"internal/sessionstate":            {layer: core, step: "S13"},
	"internal/control":                 {layer: core, step: "S13"},
	"internal/grant":                   {layer: core, step: "S13"},
	"internal/grantauth":               {layer: core, step: "S13"},
	"internal/grantauth/grantauthtest": {layer: core, step: "S13"},
	"internal/role":                    {layer: core, step: "S13"},
	"internal/brief":                   {layer: core, step: "S13"},
	"internal/channel":                 {layer: core, step: "S13"},
	"internal/inbox":                   {layer: core, step: "S13"},

	"internal/harness":         {layer: api, step: "S10"},
	"internal/harness/claude":  {layer: api + "/claude", step: "S10"},
	"internal/harness/fixture": {layer: api + "/fixture", step: "S10"},
	"internal/harness/catalog": {layer: catalog, step: "S10"},

	"internal/bridge":          {layer: tool, step: "S16"},
	"internal/bridge/endpoint": {layer: host, step: "S16"},
	"internal/wrap":            {layer: host, step: "S16"},
	"internal/alias":           {layer: host, step: "S16"},
	"internal/worktree":        {layer: host, step: "S16"},

	// What goes is judged in the layer it holds today until its step removes it.
	"internal/harness/codex":            {layer: api + "/codex", step: "S8", goes: true},
	"internal/harness/codex/gateway":    {layer: api + "/codex", step: "S8", goes: true},
	"internal/bridge/server":            {layer: tool, step: "S8", goes: true},
	"internal/harness/claude/telemetry": {layer: api + "/claude", step: "S9", goes: true},
}

// importException is a known forbidden import: the files that make it, why, and
// the step that removes it.
type importException struct {
	from, to string
	files    []string
	reason   string
	step     string
}

func (x importException) excuses(e edge) bool {
	return x.from == e.from && x.to == e.to && slices.Contains(x.files, e.file)
}

// importExceptions are the forbidden edges of the 1.x tree, from the first
// run of the test (docs/v2/stage3-steps.md#s1-the-tests-first).
var importExceptions = []importException{
	{
		from: "internal/cli", to: "internal/harness/claude",
		files:  []string{"internal/cli/granthook.go", "internal/cli/granthook_test.go"},
		reason: "the grant hook command answers Claude Code's permission hook",
		step:   "S9",
	},
	{
		from: "internal/cli", to: "internal/harness/claude/telemetry",
		files: []string{
			"internal/cli/pending.go", "internal/cli/telemetry.go", "internal/cli/turnended.go",
			"internal/cli/inbox_awaited_test.go", "internal/cli/pending_test.go", "internal/cli/turn_hold_test.go",
		},
		reason: "the telemetry receiver, the turn-start file and the stop texts",
		step:   "S9",
	},
	{
		from: "internal/cli", to: "internal/bridge/server",
		files:  []string{"internal/cli/bridge_serve.go"},
		reason: "the hidden MCP server command",
		step:   "S8",
	},
	{
		from: "internal/cli", to: "internal/harness/codex/gateway",
		files: []string{
			"internal/cli/gap_advisory_test.go", "internal/cli/gateway_integration_test.go", "internal/cli/gateway_intent_test.go",
			"internal/cli/gateway_side_test.go", "internal/cli/gateway_wire_test.go", "internal/cli/review_receipt_identity_test.go",
		},
		reason: "Codex fixtures: the gateway's turn ends driven through the CLI",
		step:   "S8",
	},
	{
		from: "internal/cli", to: "internal/harness/catalog",
		files:  []string{"internal/cli/cli_test.go"},
		reason: "the parser's tests read the registry the catalog fills; from S10 cmd/rewake passes the catalog value",
		step:   "S10",
	},
	{
		from: "internal/harness", to: "internal/bridge",
		files:  []string{"internal/harness/mailtool.go"},
		reason: "the 1.x contract carries tool values: the injected MCP server's transport",
		step:   "S8",
	},
	{
		from: "internal/channel", to: "internal/harness",
		files:  []string{"internal/channel/preview_test.go"},
		reason: "the notice text is the harness package's until it moves to the core",
		step:   "S10",
	},
	{
		from: "internal/bridge/server", to: "internal/bridge/endpoint",
		files:  []string{"internal/bridge/server/server.go", "internal/bridge/server/child_bound_test.go", "internal/bridge/server/rig_test.go"},
		reason: "the MCP server reaches the host endpoint itself",
		step:   "S8",
	},
	{
		from: "internal/bridge/server", to: "internal/cli",
		files:  []string{"internal/bridge/server/rig_test.go", "internal/bridge/server/order_marks_test.go"},
		reason: "the rig runs the CLI's read and completion",
		step:   "S8",
	},
	{
		from: "internal/bridge/server", to: "internal/harness",
		files:  []string{"internal/bridge/server/order_marks_test.go"},
		reason: "the rig completes turns through the 1.x contract",
		step:   "S8",
	},
	{
		from: "internal/alias", to: "internal/harness/catalog",
		files:  []string{"internal/alias/harness_flags_test.go"},
		reason: "the tests take a real adapter",
		step:   "S10",
	},
	{
		from: "internal/wrap", to: "internal/harness/catalog",
		files:  []string{"internal/wrap/naming_test.go"},
		reason: "the tests take a real adapter",
		step:   "S10",
	},
	{
		from: "internal/wrap", to: "internal/harness/claude",
		files:  []string{"internal/wrap/grant_resume_test.go"},
		reason: "the tests take a real adapter",
		step:   "S10",
	},
	{
		from: "internal/harness/catalog", to: "internal/harness/codex",
		files:  []string{"internal/harness/catalog/catalog.go"},
		reason: "the catalog lists Codex until it leaves",
		step:   "S8",
	},
	{
		from: "internal/harness/claude", to: "internal/harness/claude/telemetry",
		files: []string{
			"internal/harness/claude/claude.go", "internal/harness/claude/notice.go", "internal/harness/claude/plugin.go",
			"internal/harness/claude/settings.go", "internal/harness/claude/statusline.go",
			"internal/harness/claude/plugin_test.go", "internal/harness/claude/plugin_window_test.go",
		},
		reason: "the hook machinery and the status line feed the telemetry that leaves with them",
		step:   "S9",
	},
	{
		from: "internal/harness/codex", to: "internal/bridge/endpoint",
		files:  []string{"internal/harness/codex/server_mailtool.go"},
		reason: "the Codex server's mail tool reaches the endpoint",
		step:   "S8",
	},
	{
		from: "internal/harness/codex", to: "internal/worktree",
		files:  []string{"internal/harness/codex/gitmetadata_test.go"},
		reason: "a Codex fixture of a linked worktree",
		step:   "S8",
	},
	{
		from: "internal/harness/codex", to: "internal/cli",
		files:  []string{"internal/harness/codex/report_boundary_test.go"},
		reason: "a Codex notify driven through the CLI",
		step:   "S8",
	},
}

// nameException is a known harness word in a file outside the adapters. Its
// excuse records exactly which mentions of the word the file had, so a new one
// beside them fails as surely as one in a file with no entry.
type nameException struct{ file, word string }

func nameKey(m mention) nameException { return nameException{file: m.file, word: m.word} }

func (x nameException) String() string {
	return fmt.Sprintf("the name exception for %q in %s", x.word, x.file)
}

// excuse says why the findings under one key are still there, the step that
// removes them, and which they are: their number and the first twelve hex
// digits of the SHA-256 of their identities, sorted. Lines are not recorded,
// since any edit above would move them; what is said, and how often, is.
type excuse struct {
	reason, step string
	count        int
	sum          string
}

// finding is what an exception table excuses.
type finding interface {
	fmt.Stringer
	identity() string
}

// fingerprint is the count and sum an excuse records for a set of findings.
func fingerprint[F finding](found []F) (int, string) {
	var identities []string
	for _, f := range found {
		identities = append(identities, f.identity())
	}
	slices.Sort(identities)
	sum := sha256.Sum256([]byte(strings.Join(identities, "\n")))
	return len(found), hex.EncodeToString(sum[:])[:12]
}

// registryException is a known registration, by file and what it is.
type registryException struct{ file, what string }

func registryKey(r registration) registryException {
	return registryException{file: r.file, what: r.what}
}

func (x registryException) String() string {
	return fmt.Sprintf("the registry exception %q in %s", x.what, x.file)
}

// excused returns what an exception table leaves of the findings: each one no
// entry covers; each entry whose findings are no longer the ones it records,
// whether one was added, removed or replaced; then each entry that excuses
// nothing any more.
func excused[F finding, K interface {
	comparable
	fmt.Stringer
}](found []F, key func(F) K, table map[K]excuse,
) []string {
	var problems, changed, stale []string
	groups := map[K][]F{}
	for _, f := range found {
		if _, ok := table[key(f)]; !ok {
			problems = append(problems, f.String())
			continue
		}
		groups[key(f)] = append(groups[key(f)], f)
	}
	for x, why := range table {
		group, ok := groups[x]
		if !ok {
			stale = append(stale, x.String()+" matches nothing any more: remove it")
			continue
		}
		if count, sum := fingerprint(group); count != why.count || sum != why.sum {
			changed = append(changed, changedMessage(x, why, group, count, sum))
		}
	}
	slices.Sort(changed)
	slices.Sort(stale)
	return slices.Concat(problems, changed, stale)
}

func changedMessage[F finding](x fmt.Stringer, why excuse, group []F, count int, sum string) string {
	var lines []string
	for _, f := range group {
		lines = append(lines, "\t"+f.String())
	}
	return fmt.Sprintf("%s records %d (%s), and the file now has %d (%s):\n%s\n"+
		"An exception keeps what 1.x had; it admits nothing new, so a new one moves to the adapter. "+
		"Only when old ones were removed or reworded, record %d and %q.",
		x, why.count, why.sum, count, sum, strings.Join(lines, "\n"), count, sum)
}

var registryExceptions = map[registryException]excuse{
	{"internal/harness/harness.go", "package-level variable registered holds adapters ([]github.com/praline-labs/rewake/internal/harness.Harness)"}: {
		"the 1.x registry Register fills", "S10", 1, "26c51dca7432",
	},
	{"internal/harness/catalog/catalog.go", "func init in a package under adapter"}: {
		"the catalog registers every adapter from init; S10 makes it a constructor", "S10", 1, "dc886ebafbc2",
	},
	{"internal/harness/catalog/fixture.go", "func init in a package under adapter"}: {
		"the fixture joins the catalog from a tagged init, beside the others, until S10 makes the catalog a constructor", "S10", 1, "08005cc75894",
	},
	{"internal/harness/check_holder.go", "func init in a package under adapter"}: {
		"the name check's holder enters from init when the binary is re-executed as it", "S8", 1, "7bab8cf35620",
	},
}

// unmatchedImports names every file of an exception no forbidden edge comes
// from any more.
func unmatchedImports(exceptions []importException, found []edge) []string {
	var problems []string
	for _, x := range exceptions {
		for _, file := range x.files {
			if !slices.ContainsFunc(found, func(e edge) bool { return x.excuses(e) && e.file == file }) {
				problems = append(problems, fmt.Sprintf("the import exception %s -> %s for %s matches nothing any more: remove it", x.from, x.to, file))
			}
		}
	}
	return problems
}

// staleTransitions names every entry whose package is at its 2.0 path, which
// needs no entry, or gone, which needs none either, and every step that is
// not one of the build order's.
func staleTransitions(table map[string]transit, packages, steps []string) []string {
	var problems []string
	for path, entry := range table {
		if _, ok := layerOfPath(path); ok {
			problems = append(problems, fmt.Sprintf("the transition table maps %s, which is at its 2.0 path: remove the entry", path))
		} else if !slices.Contains(packages, path) {
			problems = append(problems, fmt.Sprintf("the transition table maps %s, which is gone: remove the entry", path))
		}
		if !validStep(entry.step, steps) {
			problems = append(problems, stepProblem("the transition entry of "+path, entry.step))
		}
	}
	slices.Sort(problems)
	return problems
}

// unexplained names every exception without a reason, its files or what it
// excuses, and every step that is not one of the build order's.
func unexplained(steps []string) []string {
	var problems []string
	for _, x := range importExceptions {
		what := fmt.Sprintf("the import exception %s -> %s", x.from, x.to)
		if x.reason == "" || len(x.files) == 0 {
			problems = append(problems, what+" needs its files and a reason")
		}
		if !validStep(x.step, steps) {
			problems = append(problems, stepProblem(what, x.step))
		}
	}
	problems = append(problems, unexplainedTable(nameExceptions, steps)...)
	problems = append(problems, unexplainedTable(registryExceptions, steps)...)
	slices.Sort(problems)
	return problems
}

func unexplainedTable[K interface {
	comparable
	fmt.Stringer
}](table map[K]excuse, steps []string) []string {
	var problems []string
	for x, why := range table {
		if why.reason == "" || why.count < 1 || len(why.sum) != 12 {
			problems = append(problems, x.String()+" needs a reason and the count and sum of what it excuses")
		}
		if !validStep(why.step, steps) {
			problems = append(problems, stepProblem(x.String(), why.step))
		}
	}
	return problems
}
