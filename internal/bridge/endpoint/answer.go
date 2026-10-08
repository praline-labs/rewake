package endpoint

import (
	"encoding/json"
	"strconv"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The one encoder of a transport's answers: every answer is checked whole
// against bridge.ResultCap before it leaves, and one that does not fit is
// replaced whole. What the transport hands its model is exactly this.

// ToolAnswer is a call's result as the transport hands it to the model: its
// texts in order — the child's stdout, its stderr when it printed any, its
// exit status when not zero — and whether the call failed.
type ToolAnswer struct {
	Texts   []string `json:"texts"`
	IsError bool     `json:"isError"`
}

// childAnswer is what a call answers: a child's output, or the endpoint's
// own line in place of one, which is always an error.
type childAnswer struct {
	stdout, stderr string
	code           int
	// substituted says no child's answer is in it.
	substituted bool
}

// substitute is the endpoint's own answer: one line, an error.
func substitute(line string) childAnswer {
	return childAnswer{stdout: line, substituted: true}
}

// overBound replaces an answer that would not fit. It comes only from a child
// that broke its own bound, and matches no record.
const overBound = "Rewake: the answer of this call did not fit one tool result; run the same words in the shell.\n"

func (a childAnswer) result() ToolAnswer {
	out := ToolAnswer{Texts: []string{a.stdout}, IsError: a.substituted || a.code != 0}
	if a.stderr != "" {
		out.Texts = append(out.Texts, a.stderr)
	}
	if a.code != 0 {
		out.Texts = append(out.Texts, "exit status "+strconv.Itoa(a.code))
	}
	return out
}

// encoded is the answer as it leaves: itself when it fits, overBound when not.
func (a childAnswer) encoded() ToolAnswer {
	if fits(a.result()) {
		return a.result()
	}
	return substitute(overBound).result()
}

// fits says whether an answer, encoded, is inside the bound of one result.
func fits(answer ToolAnswer) bool {
	encoded, err := json.Marshal(answer)
	return err == nil && len(encoded) <= bridge.ResultCap
}
