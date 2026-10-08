package gateway

import (
	"encoding/json"
	"time"
)

// Activity is a primary status observation, never an inference from a turn result.
func applyActivity(entry *threadObservation, raw []byte, now time.Time) {
	var activity *string
	var waiting []string
	switch kind := str(raw, "type"); kind {
	case "idle", "notLoaded", "systemError":
		value := map[string]string{"idle": "idle", "notLoaded": "not_loaded", "systemError": "system_error"}[kind]
		activity = &value
		waiting = []string{}
	case "active":
		value := "working"
		activity = &value
		flags := field(raw, "activeFlags")
		var values []string
		if len(flags) > 0 && len(flags) <= 1024 && json.Unmarshal(flags, &values) == nil && values != nil {
			waiting = []string{}
			approval, input := false, false
			valid := true
			for _, flag := range values {
				switch flag {
				case "waitingOnApproval":
					approval = true
				case "waitingOnUserInput":
					input = true
				default:
					valid = false
				}
			}
			if !valid {
				waiting = nil
			} else {
				if approval {
					waiting = append(waiting, "approval")
				}
				if input {
					waiting = append(waiting, "input")
				}
			}
		}
	}
	entry.snapshot.Activity = activity
	entry.snapshot.WaitingFor = waiting
	entry.snapshot.ActivityAt = &now
	entry.snapshot.ActivityFresh = activity != nil
}

func (c *connection) observeActivityReply(m meta, raw []byte, p pending) {
	if m.failure || p.observationGeneration != c.state.Generation {
		return
	}
	lifecycle := (p.intent || p.fork) && p.generation == c.state.Generation && m.directKnown && m.direct
	accepted := c.state.Ready && c.state.Thread == m.thread
	candidate := p.fork && c.state.fork != nil && c.state.fork.child == m.thread
	if lifecycle && (accepted || candidate) {
		if entry := c.observationEntry(m.thread); entry != nil && entry.snapshot.Activity == nil && entry.activityVersion == 0 {
			status := field(raw, "result", "thread", "status")
			if len(status) > 0 {
				applyActivity(entry, status, observedNow(&entry.snapshot))
			}
			entry.activitySerial = max(entry.activitySerial, p.observationSerial)
		}
		return
	}
	if p.method != "thread/read" || !p.metadataOnly || m.thread == "" || m.thread != p.target {
		return
	}
	entry := c.observationThread(m.thread)
	if entry == nil || p.observationActivityVersion != entry.activityVersion || p.observationSerial <= entry.activitySerial {
		return
	}
	entry.activitySerial = p.observationSerial
	applyActivity(entry, field(raw, "result", "thread", "status"), observedNow(&entry.snapshot))
}

func (c *connection) observeActivityEvent(m meta, raw []byte) {
	if m.method != "thread/status/changed" && m.method != "thread/started" {
		return
	}
	entry := c.observationThread(m.thread)
	if entry == nil {
		return
	}
	status := field(raw, "params", "status")
	if m.method == "thread/started" {
		if entry.snapshot.Activity != nil || entry.activityVersion > 0 {
			return
		}
		status = field(raw, "params", "thread", "status")
	}
	entry.activityVersion++
	applyActivity(entry, status, observedNow(&entry.snapshot))
}
