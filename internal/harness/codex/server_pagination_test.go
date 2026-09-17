package codex

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// The real loaded-list handler parses a non-null cursor as a ThreadId, even
// when the empty string looks like a convenient first-page sentinel.
func fixtureLoadedPage(ids []string, raw json.RawMessage) (any, error) {
	var params struct {
		Cursor *string `json:"cursor"`
		Limit  *int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, err
	}
	ids = append([]string{}, ids...)
	sort.Strings(ids)
	start := 0
	if len(ids) > 0 && params.Cursor != nil {
		value := *params.Cursor
		compact := strings.ReplaceAll(value, "-", "")
		_, err := hex.DecodeString(compact)
		if len(value) != 36 || len(compact) != 32 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || err != nil {
			return nil, fmt.Errorf("invalid cursor: %s", value)
		}
		start = sort.SearchStrings(ids, value)
		if start < len(ids) && ids[start] == value {
			start++
		}
	}
	limit := len(ids)
	if params.Limit != nil {
		limit = max(1, *params.Limit)
	}
	end := min(len(ids), start+limit)
	var next *string
	if end < len(ids) && end > start {
		value := ids[end-1]
		next = &value
	}
	return map[string]any{"data": ids[start:end], "nextCursor": next}, nil
}
