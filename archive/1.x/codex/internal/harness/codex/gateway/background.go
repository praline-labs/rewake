package gateway

// The pinned overview refresh uses random UUID request IDs and metadata-only
// reads; interactive selection uses the session's numeric request counter.
// This is version-specific evidence, not a general JSON-RPC ownership property.
func uuidID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, ch := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if ch != '-' {
				return false
			}
			continue
		}
		hexDigit := ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f'
		if !hexDigit {
			return false
		}
	}
	return true
}
