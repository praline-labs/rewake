package internal

// The harness words rule 2 found on its first run over the 1.x tree, file by
// file, each with the step that takes it out (docs/v2/stage3-packages.md) and
// the count and sum of the mentions it excuses (excuse). A mention added, removed
// or replaced fails until the entry says so, and an entry with none left fails,
// so the table only shrinks as the steps land.

// Why a mention is still there.
const (
	explainsByOneHarness = "a comment explains core code by one harness's behavior; reworded when the tables empty"
)

var nameExceptions = map[nameException]excuse{
	{"internal/cli/steer.go", "claude"}:               {explainsByOneHarness, "S19", 1, "88a93d4a5671"},
	{"internal/core/control/serve.go", "claude"}:      {explainsByOneHarness, "S19", 1, "35666bd41899"},
	{"internal/core/mail/held.go", "claude"}:          {explainsByOneHarness, "S19", 1, "6a12b71af3f5"},
	{"internal/core/mail/inbox.go", "claude"}:         {explainsByOneHarness, "S19", 1, "9d1f40e53eba"},
	{"internal/core/mail/kept.go", "claude"}:          {explainsByOneHarness, "S19", 1, "cdac375051ed"},
	{"internal/core/mail/window.go", "claude"}:        {explainsByOneHarness, "S19", 1, "faa4320befd2"},
	{"internal/worktree/include.go", "claude"}:        {explainsByOneHarness, "S19", 3, "a005ba09a070"},
	{"internal/worktree/include_rules.go", "claude"}:  {explainsByOneHarness, "S19", 1, "e5bfbb825bd0"},
	{"internal/worktree/name.go", "claude"}:           {explainsByOneHarness, "S19", 2, "3a201afd2d72"},
	{"internal/wrap/compaction_letters.go", "claude"}: {explainsByOneHarness, "S19", 1, "0ddfe285c6d1"},
}
