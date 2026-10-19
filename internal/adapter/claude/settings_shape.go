package claude

import (
	"encoding/json"
	"fmt"
	"strings"
)

// checkShapes refuses a key a default owns holding another kind than the
// default's — a string where Claude Code wants a boolean, an object in a
// list of names — which makes Claude Code drop the whole file. The person's
// other keys are theirs to get right.
func checkShapes(settings map[string]any, defaults []setting) error {
	for _, one := range defaults {
		parent, err := objectAt(settings, one.path, false)
		if err != nil {
			return err
		}
		if parent == nil {
			continue
		}
		name := strings.Join(one.path, ".")
		current, has := parent[one.path[len(one.path)-1]]
		switch {
		case !has:
		case !one.item:
			if kindOf(current) != kindOf(one.value) {
				return fmt.Errorf("%s is %s, not %s", name, kindOf(current), kindOf(one.value))
			}
		default:
			list, ok := current.([]any)
			if !ok {
				return fmt.Errorf("%s is %s, not a list", name, kindOf(current))
			}
			for _, value := range list {
				if kindOf(value) != kindOf(one.value) {
					return fmt.Errorf("%s holds %s, where each is %s", name, kindOf(value), kindOf(one.value))
				}
			}
		}
	}
	return nil
}

// kindOf names a decoded JSON value's kind.
func kindOf(value any) string {
	switch value.(type) {
	case bool:
		return "a boolean"
	case string:
		return "a string"
	case json.Number, float64, int:
		return "a number"
	case []any:
		return "a list"
	case map[string]any:
		return "an object"
	}
	return "null"
}
