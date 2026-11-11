package adapter

import "github.com/praline-labs/rewake/internal/core/workmode"

// WorkModesHarness supports runtime work stances. StartModeArgs fills absent
// settings for a direct launch; a worker's template is overlaid before typed
// flags are merged, so those flags remain strongest. typed lets the adapter
// remove lower-priority conflicting selections, including a model's effort
// suffix or a model scope, before that merge.
type WorkModesHarness interface {
	StartModeArgs(args []string, config workmode.Config, mode string, replace bool, typed []string) []string
}
