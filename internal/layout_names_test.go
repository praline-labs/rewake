package internal

// The harness words rule 2 found on its first run over the 1.x tree, file by
// file, each with the step that takes it out (docs/v2/stage3-packages.md) and
// the count and sum of the mentions it excuses (excuse). A mention added, removed
// or replaced fails until the entry says so, and an entry with none left fails,
// so the table only shrinks as the steps land.

// Why a mention is still there.
const (
	realAdapterIDs       = "a test or a comment takes a real adapter's id; from S10 the catalog value and the fixture supply it"
	harnessWordsInTexts  = "a help or role text names a harness; from S10 the harness words come from the adapter"
	harnessDirectories   = "the harness directories come from each adapter's ProtectedDirs from S10"
	explainsByOneHarness = "a comment explains core code by one harness's behavior; reworded when the tables empty"
)

var nameExceptions = map[nameException]excuse{
	{"internal/alias/alias_file_test.go", "claude"}:    {realAdapterIDs, "S10", 4, "891ee9c9c865"},
	{"internal/alias/command_test.go", "claude"}:       {realAdapterIDs, "S10", 16, "fe2bd2c5b3b6"},
	{"internal/alias/harness_flags_test.go", "claude"}: {realAdapterIDs, "S10", 1, "e0f23a0152d7"},
	{"internal/cli/args.go", "claude"}:                 {harnessWordsInTexts, "S10", 2, "ae90d4831642"},
	{"internal/cli/bridge_tool_test.go", "claude"}:     {realAdapterIDs, "S10", 1, "0bac7c392141"},
	{"internal/cli/cli_test.go", "claude"}:             {realAdapterIDs, "S10", 4, "c2d29210c047"},
	{"internal/cli/command_flag_test.go", "claude"}:    {realAdapterIDs, "S10", 6, "1cb6709ff94e"},
	{"internal/cli/launch.go", "claude"}:               {harnessWordsInTexts, "S10", 1, "48cb0b1e954c"},
	{"internal/cli/launch_nested_test.go", "claude"}:   {realAdapterIDs, "S10", 1, "546748e7e9b9"},
	{"internal/cli/launch_worktree_test.go", "claude"}: {realAdapterIDs, "S10", 10, "52ee4f2672bb"},
	{"internal/cli/model.go", "claude"}:                {harnessWordsInTexts, "S10", 1, "b65954e036ba"},
	{"internal/cli/read_test.go", "claude"}:            {realAdapterIDs, "S10", 1, "0bac7c392141"},
	{"internal/cli/registry.go", "claude"}:             {harnessWordsInTexts, "S10", 7, "17815d02d540"},
	{"internal/cli/registry_send.go", ".codex"}:        {harnessDirectories, "S10", 1, "2e213450bb2f"},
	{"internal/cli/registry_send.go", "claude"}:        {harnessWordsInTexts, "S10", 3, "a2951febcdc9"},
	{"internal/cli/registry_send.go", ".agents"}:       {harnessDirectories, "S10", 1, "2e213450bb2f"},
	{"internal/cli/role_test.go", "claude"}:            {realAdapterIDs, "S10", 2, "fac1f46fa9dd"},
	{"internal/cli/rooms_test.go", "claude"}:           {realAdapterIDs, "S10", 3, "9472489bb6cd"},
	{"internal/cli/sessions.go", "claude"}:             {harnessWordsInTexts, "S10", 1, "e650a5d9965d"},
	{"internal/cli/sessions_test.go", "claude"}:        {realAdapterIDs, "S10", 2, "29fc6a495088"},
	{"internal/cli/steer.go", "claude"}:                {explainsByOneHarness, "S19", 1, "88a93d4a5671"},
	{"internal/cli/steer_test.go", "claude"}:           {realAdapterIDs, "S10", 1, "65bcc0027c9c"},
	{"internal/cli/thread_report_test.go", "claude"}:   {realAdapterIDs, "S10", 1, "0bac7c392141"},
	{"internal/cli/worktree.go", "claude"}:             {harnessWordsInTexts, "S10", 1, "9861882ce071"},
	{"internal/control/serve.go", "claude"}:            {explainsByOneHarness, "S19", 1, "35666bd41899"},
	{"internal/grant/rules.go", ".codex"}:              {harnessDirectories, "S10", 1, "e419ea01416c"},
	{"internal/grant/rules.go", "claude"}:              {harnessDirectories, "S10", 2, "ba6bf582fdd9"},
	{"internal/grant/rules.go", ".agents"}:             {harnessDirectories, "S10", 1, "00264a0b6a6f"},
	{"internal/grant/rules_live_test.go", ".codex"}:    {harnessDirectories, "S10", 2, "35b6ad601a67"},
	{"internal/grant/rules_live_test.go", "claude"}:    {harnessDirectories, "S10", 3, "dd0e56719628"},
	{"internal/grant/rules_live_test.go", ".agents"}:   {harnessDirectories, "S10", 2, "4e727ba3f5fd"},
	{"internal/grant/rules_shield_test.go", ".codex"}:  {harnessDirectories, "S10", 1, "39aa401b2200"},
	{"internal/grant/rules_shield_test.go", "claude"}:  {harnessDirectories, "S10", 1, "02a9ffad5a79"},
	{"internal/grant/rules_shield_test.go", ".agents"}: {harnessDirectories, "S10", 1, "dff5d33d1653"},
	{"internal/grant/rules_test.go", "claude"}:         {harnessDirectories, "S10", 1, "3771568a0f25"},
	{"internal/inbox/held.go", "claude"}:               {explainsByOneHarness, "S19", 1, "6a12b71af3f5"},
	{"internal/inbox/inbox.go", "claude"}:              {explainsByOneHarness, "S19", 1, "5d592dd0c53c"},
	{"internal/inbox/kept.go", "claude"}:               {explainsByOneHarness, "S19", 1, "cdac375051ed"},
	{"internal/inbox/window.go", "claude"}:             {explainsByOneHarness, "S19", 1, "faa4320befd2"},
	{"internal/registry/registry.go", "claude"}:        {realAdapterIDs, "S10", 1, "1a2c9b51bcc7"},
	{"internal/registry/registry_test.go", "claude"}:   {realAdapterIDs, "S10", 9, "d46487a4c52c"},
	{"internal/role/playbook.go", "claude"}:            {harnessWordsInTexts, "S10", 2, "f198628550e8"},
	{"internal/role/playbook_main.go", ".codex"}:       {harnessDirectories, "S10", 1, "b2f9a613fdd8"},
	{"internal/role/playbook_main.go", ".agents"}:      {harnessDirectories, "S10", 1, "b2f9a613fdd8"},
	{"internal/role/playbook_main.go", "claude"}:       {harnessWordsInTexts, "S10", 3, "42278cb1eadb"},
	{"internal/role/role.go", "claude"}:                {harnessWordsInTexts, "S10", 1, "6897abe791d9"},
	{"internal/state/state_test.go", "claude"}:         {realAdapterIDs, "S10", 1, "964b30cc6fc0"},
	{"internal/worktree/include.go", "claude"}:         {explainsByOneHarness, "S19", 3, "a005ba09a070"},
	{"internal/worktree/include_rules.go", "claude"}:   {explainsByOneHarness, "S19", 1, "e5bfbb825bd0"},
	{"internal/worktree/name.go", "claude"}:            {explainsByOneHarness, "S19", 2, "3a201afd2d72"},
	{"internal/wrap/compaction_letters.go", "claude"}:  {explainsByOneHarness, "S19", 1, "0ddfe285c6d1"},
	{"internal/wrap/ownership_test.go", "claude"}:      {realAdapterIDs, "S10", 1, "0bac7c392141"},
	{"internal/wrap/thread_test.go", "claude"}:         {realAdapterIDs, "S10", 2, "4da672537616"},
	{"internal/wrap/wrap_test.go", "claude"}:           {realAdapterIDs, "S10", 1, "f714ce850ec4"},
}
