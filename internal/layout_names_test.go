package internal

// The harness words rule 2 found on its first run over the 1.x tree, file by
// file, each with the step that takes it out (docs/v2/stage3-packages.md) and
// the count and sum of the mentions it excuses (excuse). A mention added, removed
// or replaced fails until the entry says so, and an entry with none left fails,
// so the table only shrinks as the steps land.

// Why a mention is still there.
const (
	harnessWordsInTexts  = "a help or role text names a harness; from S10 the harness words come from the adapter"
	harnessDirectories   = "the harness directories come from each adapter's ProtectedDirs from S10"
	explainsByOneHarness = "a comment explains core code by one harness's behavior; reworded when the tables empty"
)

var nameExceptions = map[nameException]excuse{
	{"internal/cli/args.go", "claude"}:                {harnessWordsInTexts, "S10", 2, "ae90d4831642"},
	{"internal/cli/launch.go", "claude"}:              {harnessWordsInTexts, "S10", 1, "48cb0b1e954c"},
	{"internal/cli/registry.go", "claude"}:            {harnessWordsInTexts, "S10", 7, "17815d02d540"},
	{"internal/cli/registry_send.go", ".codex"}:       {harnessDirectories, "S10", 1, "2e213450bb2f"},
	{"internal/cli/registry_send.go", "claude"}:       {harnessWordsInTexts, "S10", 3, "a2951febcdc9"},
	{"internal/cli/registry_send.go", ".agents"}:      {harnessDirectories, "S10", 1, "2e213450bb2f"},
	{"internal/cli/sessions.go", "claude"}:            {harnessWordsInTexts, "S10", 1, "e650a5d9965d"},
	{"internal/cli/steer.go", "claude"}:               {explainsByOneHarness, "S19", 1, "88a93d4a5671"},
	{"internal/cli/worktree.go", "claude"}:            {harnessWordsInTexts, "S10", 1, "9861882ce071"},
	{"internal/control/serve.go", "claude"}:           {explainsByOneHarness, "S19", 1, "35666bd41899"},
	{"internal/inbox/held.go", "claude"}:              {explainsByOneHarness, "S19", 1, "6a12b71af3f5"},
	{"internal/inbox/inbox.go", "claude"}:             {explainsByOneHarness, "S19", 1, "5d592dd0c53c"},
	{"internal/inbox/kept.go", "claude"}:              {explainsByOneHarness, "S19", 1, "cdac375051ed"},
	{"internal/inbox/window.go", "claude"}:            {explainsByOneHarness, "S19", 1, "faa4320befd2"},
	{"internal/role/playbook.go", "claude"}:           {harnessWordsInTexts, "S10", 2, "f198628550e8"},
	{"internal/role/playbook_main.go", ".codex"}:      {harnessDirectories, "S10", 1, "b2f9a613fdd8"},
	{"internal/role/playbook_main.go", ".agents"}:     {harnessDirectories, "S10", 1, "b2f9a613fdd8"},
	{"internal/role/playbook_main.go", "claude"}:      {harnessWordsInTexts, "S10", 3, "42278cb1eadb"},
	{"internal/role/role.go", "claude"}:               {harnessWordsInTexts, "S10", 1, "6897abe791d9"},
	{"internal/worktree/include.go", "claude"}:        {explainsByOneHarness, "S19", 3, "a005ba09a070"},
	{"internal/worktree/include_rules.go", "claude"}:  {explainsByOneHarness, "S19", 1, "e5bfbb825bd0"},
	{"internal/worktree/name.go", "claude"}:           {explainsByOneHarness, "S19", 2, "3a201afd2d72"},
	{"internal/wrap/compaction_letters.go", "claude"}: {explainsByOneHarness, "S19", 1, "0ddfe285c6d1"},
}
