package endpoint

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The per-call limits (docs/archive-1.x/mail-bridge-launch.md#claude-code, rule 5): every
// short string over the characters a timeout might be written in, judged by an
// oracle that reads numbers without the check's parser.

// spanOracle is a call's deadline from a timeout as the rule states it: only
// decimal digits within 64 bits; under 7000 ms refused; otherwise the timeout less 2 s, at
// most 25 s.
func spanOracle(value *string) (time.Duration, string) {
	if value == nil {
		return 25 * time.Second, ""
	}
	if *value == "" || strings.Trim(*value, "0123456789") != "" {
		return 0, "not a whole number"
	}
	millis, _ := new(big.Int).SetString(*value, 10)
	if millis.BitLen() > 64 {
		return 0, "not a whole number"
	}
	if millis.Cmp(big.NewInt(7000)) < 0 {
		return 0, "under five seconds"
	}
	if millis.Cmp(big.NewInt(27000)) >= 0 {
		return 25 * time.Second, ""
	}
	return time.Duration(millis.Int64())*time.Millisecond - 2*time.Second, ""
}

// timeoutWords are every string up to five characters over an alphabet that
// holds digits on both sides of each boundary, signs, a space and a point,
// and the written forms the alphabet cannot reach.
func timeoutWords() []string {
	alphabet := []string{"0", "2", "7", "9", "+", "-", " ", "."}
	words := []string{""}
	level := []string{""}
	for length := 1; length <= 5; length++ {
		var next []string
		for _, prefix := range level {
			for _, letter := range alphabet {
				next = append(next, prefix+letter)
			}
		}
		words = append(words, next...)
		level = next
	}
	return append(words,
		"6999", "7000", "7001", "26999", "27000", "27001", "007000",
		"18446744073709551615", "18446744073709551616", "99999999999999999999999",
		"7e3", "0x1B58", "７０００", "7000\n", "\t7000", "30s", "١٢٣٤٥",
	)
}

func TestEveryWrittenTimeoutGivesItsSpan(t *testing.T) {
	refusals := map[string]string{
		"not a whole number": "MCP_TOOL_TIMEOUT is not a whole number of milliseconds",
		"under five seconds": "leaves under five seconds",
	}
	words := timeoutWords()
	counted := map[string]int{}
	check := func(value *string) {
		want, wantRefusal := spanOracle(value)
		got, refusal := callSpan(&HookLimits{Timeout: value})
		label := "unset"
		if value != nil {
			label = *value
		}
		switch {
		case wantRefusal == "" && (refusal != "" || got != want):
			t.Fatalf("%q: got %v %q, want %v", label, got, refusal, want)
		case wantRefusal != "" && !strings.Contains(refusal, refusals[wantRefusal]):
			t.Fatalf("%q: got %v %q, want refused: %s", label, got, refusal, wantRefusal)
		case wantRefusal != "" && got != 0:
			t.Fatalf("%q: a refused call has a deadline %v", label, got)
		}
		if refusal != "" && !strings.Contains(refusal, "in the shell") {
			t.Fatalf("%q: the refusal does not point to the shell: %s", label, refusal)
		}
		if wantRefusal == "" && (got < 5*time.Second || got > 25*time.Second) {
			t.Fatalf("%q: a deadline %v outside five to twenty-five seconds", label, got)
		}
		counted[wantRefusal]++
	}
	check(nil)
	for _, word := range words {
		check(&word)
	}
	t.Logf("%d timeouts: %d spans, %d not numbers, %d too short", len(words)+1, counted[""], counted["not a whole number"], counted["under five seconds"])
	for _, kind := range []string{"", "not a whole number", "under five seconds"} {
		if counted[kind] == 0 {
			t.Fatalf("the space reached no case of %q", kind)
		}
	}
}

// limitsSpace are the snapshots a hook may send: none, empty, and each
// setting at the default, raised, lowered or written badly.
func limitsSpace() []*HookLimits {
	values := []*string{nil, ptr("30000"), ptr("7000"), ptr("x"), ptr(""), ptr("2047"), ptr("2048"), ptr("+2048"), ptr("2048.0")}
	space := []*HookLimits{nil}
	for _, timeout := range values {
		for _, output := range values {
			space = append(space, &HookLimits{Timeout: timeout, Output: output})
		}
	}
	return space
}

func ptr(value string) *string { return &value }

func TestAReadIsAcknowledgedOnlyOnProvenAgreeingDefaultLimits(t *testing.T) {
	space := limitsSpace()
	cases, allowed := 0, 0
	// The bound of 2.1.284 (docs/archive-1.x/mail-bridge-version.md), or none.
	whole := map[string]bool{"30000": true, "7000": true, "2048": true}
	for _, bound := range []int64{0, 2048} {
		for _, proven := range []bool{false, true} {
			for _, pre := range space {
				for _, post := range space {
					e := &Endpoint{cfg: Config{Transport: bridge.ClaudeTransport, LimitsProven: proven, OutputBound: bound}, calls: newCalls()}
					e.calls.byCall["call"] = &call{observed: &observation{limits: pre}}
					keeps := pre != nil && (pre.Output == nil || bound > 0 && whole[*pre.Output])
					want := pre == nil && post == nil ||
						proven && pre != nil && post != nil && keeps &&
							sameValue(pre.Output, post.Output) && sameValue(pre.Timeout, post.Timeout)
					if got := e.limitsAllow("call", post); got != want {
						t.Fatalf("bound %d proven %v pre %s post %s: got %v", bound, proven, show(pre), show(post), got)
					}
					cases++
					if want {
						allowed++
					}
				}
			}
		}
	}
	t.Logf("%d snapshot pairs, %d acknowledge", cases, allowed)
}

func sameValue(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func show(limits *HookLimits) string {
	if limits == nil {
		return "none"
	}
	word := func(value *string) string {
		if value == nil {
			return "unset"
		}
		return "\"" + *value + "\""
	}
	return "{timeout " + word(limits.Timeout) + ", output " + word(limits.Output) + "}"
}
