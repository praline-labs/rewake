package pi

import "encoding/json"

// A long event comes in parts of its serialized text (stream.ts), put
// together here.

type parts struct {
	Of   int    `json:"of"`
	Next int    `json:"next"`
	Text string `json:"text"`
}

// maxParts bounds the parts of one event, the stream's bound of an event.
const maxParts = 16

// part puts a long event together. An event that cannot be — a part out
// of place, more parts than an event's bound, a text that is no event — is
// lost, and the model resyncs; the rest of an event already lost is
// passed over, since what lost its start said so.
func (t *translator) part(e raw) (raw, bool) {
	if e.Part == 0 {
		if t.Parts != nil {
			t.broken()
		}
		if e.Of <= 0 || e.Of > maxParts {
			t.lost(raw{Events: 1})
			return raw{}, false
		}
		t.Parts = &parts{Of: e.Of}
	}
	if t.Parts == nil {
		return raw{}, false
	}
	if e.Part != t.Parts.Next || e.Of != t.Parts.Of {
		t.broken()
		return raw{}, false
	}
	t.Parts.Text += e.Text
	t.Parts.Next++
	if t.Parts.Next < t.Parts.Of {
		return raw{}, false
	}
	var whole raw
	err := json.Unmarshal([]byte(t.Parts.Text), &whole)
	t.Parts = nil
	if err != nil || whole.Event == "part" || whole.Event == "" {
		t.lost(raw{Events: 1})
		return raw{}, false
	}
	return whole, true
}

// broken drops the event being put together, lost.
func (t *translator) broken() {
	t.Parts = nil
	t.lost(raw{Events: 1})
}
