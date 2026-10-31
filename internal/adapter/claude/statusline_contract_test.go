package claude

import (
	"testing"
)

// The run's line is rewake's command on rewake's interval, whatever the
// person's settings name: it shadows theirs, which is off in the run.
func TestTheRunsStatusLineIsRewakes(t *testing.T) {
	setting := statusLineSetting("/opt/it's/rewake")
	if setting["type"] != "command" || setting["command"] != `'/opt/it'\''s/rewake' statusline` || setting["refreshInterval"] != statusLineRefresh {
		t.Fatalf("the run's line: %v", setting)
	}
}
