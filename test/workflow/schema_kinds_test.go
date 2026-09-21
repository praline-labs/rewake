package workflow

import "fmt"

// What a JSON value is, and whether it is the kind a schema asked for. Kept
// apart from the walker so that file stays about the register and the rules,
// not about Go's decoding of JSON.

func matchesAny(kinds []string, value any) bool {
	for _, kind := range kinds {
		if matches(kind, value) {
			return true
		}
	}
	return false
}

func matches(kind string, value any) bool {
	switch kind {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && number == float64(int64(number))
	case "null":
		return value == nil
	}
	return false
}

func kindOf(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		if typed == float64(int64(typed)) {
			return "integer"
		}
		return "number"
	}
	return fmt.Sprintf("%T", value)
}
