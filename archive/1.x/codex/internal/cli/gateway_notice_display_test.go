package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// Consume the actual new downstream frame explicitly, so an extra or misrouted
// frame cannot silently replace a later native lifecycle reply in these tests.
func wireNoticeDisplay(t *testing.T, ui *integrationWire, thread, turn string) {
	t.Helper()
	value, err := ui.read()
	if err != nil {
		t.Fatal(err)
	}
	var method string
	var params struct {
		Thread    string `json:"threadId"`
		Turn      string `json:"turnId"`
		Completed int64  `json:"completedAtMs"`
		Item      struct {
			Kind    string `json:"type"`
			ID      string `json:"id"`
			Command string `json:"command"`
			Source  string `json:"source"`
			Status  string `json:"status"`
		} `json:"item"`
	}
	if json.Unmarshal(value["method"], &method) != nil || method != "item/completed" || json.Unmarshal(value["params"], &params) != nil {
		t.Fatalf("missing display-only completion: %s", value)
	}
	if params.Thread != thread || params.Turn != turn || params.Completed <= 0 || params.Item.Kind != "commandExecution" || params.Item.Source != "agent" || params.Item.Status != "completed" || params.Item.Command != "rewake notice --display-only" || !strings.HasPrefix(params.Item.ID, "rewake-notice-display-") {
		t.Fatalf("wrong display scope or payload: %+v", params)
	}
}
