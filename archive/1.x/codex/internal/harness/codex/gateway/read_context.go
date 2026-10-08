package gateway

import (
	"bytes"
	"encoding/json"
)

// A serialization shape is evidence, unlike v2's collapsed true-only log flag.
func valueShape(raw []byte) string {
	raw = bytes.TrimSpace(raw)
	switch string(raw) {
	case "":
		return "omitted"
	case "null":
		return "null"
	case "false":
		return "false"
	case "true":
		return "true"
	}
	if raw[0] == '{' {
		return "object"
	}
	if raw[0] == '[' {
		return "array"
	}
	return "scalar"
}

func metadataRead(m meta) bool {
	return m.paramsShape == "object" && (m.readShape == "omitted" || m.readShape == "false")
}

func (s *state) readContext(m meta) string {
	if m.method != "thread/read" {
		return ""
	}
	if m.thread == "" {
		return "unclassified-target"
	}
	if m.thread == s.Thread {
		return "accepted-thread"
	}
	if !metadataRead(m) {
		return "unclassified-shape"
	}
	if uuidID(m.idText) {
		return "overview"
	}
	if m.numeric && s.backfill[m.thread] {
		return "resume-backfill"
	}
	return "unclassified-context"
}

// Only a correlated thread/loaded/list response is inspected as a bounded ID list.
// The list does not select a destination: the accepted resume reply already did.
func loadedIDs(raw []byte) ([]string, bool) {
	data := field(raw, "result", "data")
	cursor := field(raw, "result", "nextCursor")
	if len(data) == 0 || data[0] != '[' || len(data) > 16<<10 {
		return nil, false
	}
	if len(cursor) > 0 && !bytes.Equal(cursor, []byte("null")) {
		return nil, false
	}
	var ids []string
	if json.Unmarshal(data, &ids) != nil || len(ids) > 128 {
		return nil, false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || len(id) > 128 || seen[id] {
			return nil, false
		}
		seen[id] = true
	}
	return ids, true
}

func (s *state) acceptBackfill(m meta, p pending) {
	if !p.backfill || p.readSerial != s.readSerial || m.failure || !m.loadedValid || p.generation != s.Generation || !s.Ready {
		return
	}
	found := false
	for _, id := range m.loaded {
		if id == s.Thread {
			found = true
		}
	}
	if !found {
		return
	}
	s.backfill = map[string]bool{}
	for _, id := range m.loaded {
		if id != s.Thread {
			s.backfill[id] = true
		}
	}
}
