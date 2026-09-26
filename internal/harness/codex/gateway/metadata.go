package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
)

// Unknown values are only traversed as JSON syntax; their strings are never decoded.
// The caller retains the original bytes for unchanged forwarding, not archival.
func skipValue(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\n' || b[i] == '\r' || b[i] == '\t') {
		i++
	}
	if i >= len(b) {
		return i
	}
	if b[i] == '"' {
		i++
		for i < len(b) {
			// Large opaque strings are common in native replies. Scan to the next
			// quote without visiting every payload byte in each metadata lookup.
			next := bytes.IndexByte(b[i:], '"')
			if next < 0 {
				return len(b)
			}
			i += next
			escapes := 0
			for j := i - 1; j >= 0 && b[j] == '\\'; j-- {
				escapes++
			}
			if escapes%2 == 0 {
				return i + 1
			}
			i++
		}
		return i
	}
	if b[i] == '{' || b[i] == '[' {
		end := byte('}')
		if b[i] == '[' {
			end = ']'
		}
		i++
		for i < len(b) {
			if b[i] == end {
				return i + 1
			}
			if b[i] == ',' || b[i] == ':' || b[i] == ' ' || b[i] == '\n' || b[i] == '\r' || b[i] == '\t' {
				i++
				continue
			}
			i = skipValue(b, i)
		}
		return i
	}
	for i < len(b) && !bytes.ContainsRune([]byte(",]} \n\r\t"), rune(b[i])) {
		i++
	}
	return i
}

func field(b []byte, keys ...string) []byte {
	b = bytes.TrimSpace(b)
	if len(keys) == 0 {
		return b
	}
	if len(b) < 2 || b[0] != '{' {
		return nil
	}
	for i := 1; i < len(b)-1; {
		for i < len(b) && bytes.ContainsRune([]byte(", \n\r\t"), rune(b[i])) {
			i++
		}
		if i >= len(b) || b[i] != '"' {
			return nil
		}
		end := skipValue(b, i)
		var key string
		// Object keys are routing structure, never user values.
		if end-i <= 128 {
			_ = json.Unmarshal(b[i:end], &key)
		}
		i = end
		for i < len(b) && b[i] != ':' {
			i++
		}
		i++
		start := i
		end = skipValue(b, i)
		if key == keys[0] {
			return field(b[start:end], keys[1:]...)
		}
		i = end
	}
	return nil
}

func str(b []byte, keys ...string) string {
	raw := field(b, keys...)
	if len(raw) > 512 {
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func present(b []byte, keys ...string) bool {
	v := field(b, keys...)
	return len(v) > 0 && !bytes.Equal(v, []byte("null"))
}

func boolValue(b []byte, keys ...string) bool { return bytes.Equal(field(b, keys...), []byte("true")) }

type meta struct {
	// sent is a request's place in the order of writes (operations.sent).
	sent                                                 uint64
	readThrough                                          *uint64
	resultObject                                         bool
	startupFork, permissions                             bool
	detachStatus                                         string
	reconnect                                            bool
	environments                                         json.RawMessage
	refusal                                              string
	readShape, paramsShape, readClass                    string
	loaded                                               []string
	loadedValid                                          bool
	method, id, idText, thread, status, turn, source     string
	numeric, config, roots, direct, directKnown, failure bool
	includeTurnsKnown, includeTurns                      bool
	// tuiConfig and byID are the shape of the terminal's own lifecycle
	// requests where the workspace roots no longer mark them (tuiConfig).
	tuiConfig, byID bool
}

func project(raw []byte) (meta, error) {
	var m meta
	if len(raw) > maxMessage {
		return m, &sizeError{Stage: "projection-message", Size: uint64(len(raw)), Limit: maxMessage}
	}
	if !json.Valid(raw) || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return m, errors.New("invalid RPC object")
	}
	if err := uniqueControlFields(raw); err != nil {
		return m, err
	}
	m.method = str(raw, "method")
	if m.method == "thread/read" {
		m.readShape = valueShape(field(raw, "params", "includeTurns"))
		m.paramsShape = valueShape(field(raw, "params"))
	}
	id := field(raw, "id")
	if len(id) > 0 {
		if len(id) > 256 {
			return m, errors.New("request id too long")
		}
		if id[0] == '"' {
			m.idText = str(id)
			m.id = "s:" + m.idText
		} else {
			if _, err := strconv.ParseInt(string(id), 10, 64); err != nil {
				return m, errors.New("unsupported request id")
			}
			m.numeric = true
			m.id = "n:" + string(id)
		}
	}
	m.thread = str(raw, "params", "threadId")
	m.includeTurnsKnown = present(raw, "params", "includeTurns")
	m.includeTurns = boolValue(raw, "params", "includeTurns")
	m.config = present(raw, "params", "config")
	// legacy(codex <0.157.1): the roots and the permissions mark the terminal's requests on 0.155.1 only; remove when 0.155.1 is no longer supported
	m.roots = present(raw, "params", "runtimeWorkspaceRoots")
	m.permissions = present(raw, "params", "permissions")
	m.tuiConfig = tuiConfig(raw)
	m.byID = !present(raw, "params", "history") && !present(raw, "params", "path")
	m.source = str(raw, "params", "threadSource")
	m.status = str(raw, "params", "status", "type")
	m.turn = str(raw, "params", "turnId")
	if m.turn == "" {
		m.turn = str(raw, "params", "turn", "id")
	}
	if m.method == "turn/completed" {
		m.status = str(raw, "params", "turn", "status")
	}
	if m.method == "thread/started" {
		m.thread = str(raw, "params", "thread", "id")
		m.status = str(raw, "params", "thread", "status", "type")
	}
	m.failure = present(raw, "error")
	if m.method == "" {
		m.resultObject = valueShape(field(raw, "result")) == "object"
		m.detachStatus = str(raw, "result", "status")
		m.thread = str(raw, "result", "thread", "id")
		m.status = str(raw, "result", "thread", "status", "type")
		m.directKnown = present(raw, "result", "thread", "canAcceptDirectInput")
		m.direct = boolValue(raw, "result", "thread", "canAcceptDirectInput")
		if !m.directKnown {
			m.directKnown = present(raw, "result", "canAcceptDirectInput")
			m.direct = boolValue(raw, "result", "canAcceptDirectInput")
		}
		m.turn = str(raw, "result", "turn", "id")
		if m.turn == "" {
			m.turn = str(raw, "result", "turnId")
		}
	}
	return m, nil
}

// tuiConfig says a request carries the configuration the terminal's own builder
// writes into every start, resume and fork it sends: an object holding
// web_search, a string of the four modes, beside whatever else the launch set.
// Until 0.155.1 the workspace roots marked those requests; a remote terminal of
// 0.157.1 sends none, and the permissions are absent on both. So this is what
// is left to tell the terminal's selection from anything else — a version's
// habit, read in its source (tui/src/app_server_session.rs,
// config_request_overrides_from_config), not a promise of the protocol. A
// configuration that is merely present is not it: a reconnect's carries the
// defaults only. The helpers carry web_search too — the temporary helper its own
// configuration with web_search "disabled" (temporary_structured_request.rs),
// the dynamic one the builder's on a start — so only their request-id prefixes
// tell them apart (isHelper).
func tuiConfig(raw []byte) bool {
	config := field(raw, "params", "config")
	if len(config) == 0 || config[0] != '{' {
		return false
	}
	switch str(config, "web_search") {
	case "disabled", "cached", "indexed", "live":
		return true
	}
	return false
}
