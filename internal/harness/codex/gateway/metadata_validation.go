package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Duplicate routing fields could make first-key projection disagree with the
// native parser's last-key choice. Reject them before forwarding either way.
func uniqueControlFields(raw []byte) error {
	for _, path := range [][]string{nil, {"params"}, {"result"}, {"result", "thread"}, {"result", "turn"}, {"params", "thread"}, {"params", "status"}, {"params", "turn"}, {"params", "item"}} {
		b := field(raw, path...)
		if len(b) == 0 || b[0] != '{' {
			continue
		}
		seen := map[string]bool{}
		for i := 1; i < len(b)-1; {
			for i < len(b) && bytes.ContainsRune([]byte(", \n\r\t"), rune(b[i])) {
				i++
			}
			if i >= len(b) || b[i] != '"' {
				break
			}
			end := skipValue(b, i)
			var key string
			if end-i <= 128 {
				_ = json.Unmarshal(b[i:end], &key)
			}
			i = end
			for i < len(b) && b[i] != ':' {
				i++
			}
			i++
			i = skipValue(b, i)
			switch key {
			case "id", "method", "params", "result", "error", "thread", "threadId", "turn", "turnId", "status", "type", "threadSource", "config", "runtimeWorkspaceRoots", "permissions", "canAcceptDirectInput", "includeTurns", "data", "nextCursor":
				if seen[key] {
					return errors.New("duplicate routing field")
				}
				seen[key] = true
			}
		}
	}
	return nil
}
