package gateway

import (
	"encoding/json"
	"math"
	"strconv"
	"unicode"
)

func telemetryInteger(raw []byte) *int64 {
	if len(raw) == 0 || len(raw) > 20 {
		return nil
	}
	value, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || value < 0 {
		return nil
	}
	return &value
}

func telemetryLabel(raw []byte) *string {
	if len(raw) == 0 || len(raw) > 512 {
		return nil
	}
	var value *string
	if json.Unmarshal(raw, &value) != nil || value == nil || *value == "" {
		return nil
	}
	for _, ch := range *value {
		if unicode.IsControl(ch) {
			return nil
		}
	}
	return value
}

// The native UI rounds remaining percent after its baseline adjustment; filling
// is the complement, not a separately rounded used/window ratio.
func contextFill(used, window *int64) *int {
	if used == nil || window == nil || *used < 0 || *window < 0 {
		return nil
	}
	filled := 100
	if *window > 12000 {
		available := float64(*window - 12000)
		occupied := float64(max(*used-12000, 0))
		remaining := math.Round(100 * max(available-occupied, 0) / available)
		filled = 100 - int(remaining)
	}
	return &filled
}

// Only scalar protocol metadata is inspected. Historical items in replies never
// enter the live compaction path, and request proposals never become settings.
func (c *connection) observeServer(m meta, raw []byte, p pending, correlated bool) {
	if !c.owner.owns(c) {
		return
	}
	if m.method == "" && correlated {
		c.observeSettingsReply(m, raw, p)
		c.observeActivityReply(m, raw, p)
	}
	if m.id != "" {
		c.syncObservation()
		return
	}
	c.observeActivityEvent(m, raw)
	switch m.method {
	case "thread/tokenUsage/updated":
		if entry := c.observationThread(m.thread); entry != nil && m.turn != "" && !entry.awaitingNewTurn && (entry.usageTurn == "" || entry.usageTurn == m.turn) {
			now := observedNow(&entry.snapshot)
			entry.snapshot.ContextUsed = telemetryInteger(field(raw, "params", "tokenUsage", "last", "totalTokens"))
			entry.snapshot.ContextWindow = telemetryInteger(field(raw, "params", "tokenUsage", "modelContextWindow"))
			entry.snapshot.FilledPercent = contextFill(entry.snapshot.ContextUsed, entry.snapshot.ContextWindow)
			entry.snapshot.ContextAt = &now
			entry.contextValid = true
			entry.snapshot.ContextFresh = !entry.settingsUnresolved()
		}
	case "turn/started":
		if entry := c.observationThread(m.thread); entry != nil && m.turn != "" {
			entry.awaitingNewTurn = false
			entry.usageTurn = m.turn
		}
	case "thread/settings/updated":
		if entry := c.observationThread(m.thread); entry != nil {
			now := observedNow(&entry.snapshot)
			entry.settingsFence++
			applySettings(entry, field(raw, "params", "threadSettings", "model"), field(raw, "params", "threadSettings", "effort"), now, true)
		}
	case "turn/completed":
		if m.turn != "" && (m.status == "completed" || m.status == "failed" || m.status == "interrupted") {
			if entry := c.observationThread(m.thread); entry != nil {
				observedNow(&entry.snapshot)
				c.recordCompaction(entry, compactionObservation{thread: m.thread, turn: m.turn, terminal: true})
			}
		}
	case "item/started", "item/completed":
		if str(raw, "params", "item", "type") == "contextCompaction" {
			c.observeCompaction(m.thread, str(raw, "params", "item", "id"), m.turn, m.method == "item/completed")
		}
	}
	c.syncObservation()
}
