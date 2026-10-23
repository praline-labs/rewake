package claude

import "testing"

func TestWindowTokens(t *testing.T) {
	for value, want := range map[string]int64{"300k": 300_000, "300K": 300_000, "1M": 1_000_000, "250000": 250_000, "auto": 0, "": 0, "5k": 0, "2M": 0} {
		if got := windowTokens(value); got != want {
			t.Errorf("windowTokens(%q) = %d, want %d", value, got, want)
		}
	}
}
