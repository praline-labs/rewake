package workflow

// Tool calls of this column's session, and the permission hooks they pass
// through.
//
// A directory granted to a Claude Code session reaches it through its
// permission hooks (docs/grants.md#claude-code), so a case about a grant needs
// a session that calls tools and asks those hooks the way the harness does. The
// session here is told which calls to make by the mail itself: a line
// `tool <Write|Read|Bash> <path>` in what it read is one call, made in order
// after the mail is read and before the turn ends. The session keeps its
// working directories — the one it runs in and those a hook added — and a call
// runs, is denied or goes to a person by the rules seen live on 2.1.280 in
// acceptEdits (docs/research-claude-actions.md#a-directory-given-to-a-running-session):
//
//   - PreToolUse runs first, for a tool its matcher names; deny ends the call,
//     ask sends it to PermissionRequest even where it would have run;
//   - a write outside every working directory raises PermissionRequest with a
//     suggestion to add the directory it lies in, for the session;
//   - allow runs the call and applies the answer's addDirectories and
//     removeDirectories; silence leaves the call to a person, who in this
//     fixture never answers.
//
// Every call is recorded as a turn event of kind "tool": the tool, the path,
// the outcome, and what the answer added or removed.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// shimTools makes the session make the tool calls its mail names.
const shimTools = "RW_SHIM_TOOLS"

// shimPermissionMode is the mode the session runs in: acceptEdits, where
// writes inside the working directories run without asking, as in the probe.
const shimPermissionMode = "acceptEdits"

// toolCalls are the calls a text names, in order.
func toolCalls(text string) [][2]string {
	var calls [][2]string
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "tool" {
			calls = append(calls, [2]string{fields[1], fields[2]})
		}
	}
	return calls
}

// playTools makes the calls the mail named, when the session is asked to.
func (s *claudeSession) playTools(turn, text string) {
	if os.Getenv(shimTools) == "" {
		return
	}
	for _, call := range toolCalls(text) {
		detail := call[0] + " " + call[1] + " " + s.toolCall(call[0], call[1])
		(&shimSession{}).recordTurnEvent("tool", turn, detail)
	}
}

// toolCall makes one call and answers its outcome: ran, denied, prompted, or
// refused with why, each followed by what an answer added and removed.
func (s *claudeSession) toolCall(tool, path string) string {
	var input map[string]string
	switch tool {
	case "Write":
		input = map[string]string{"file_path": path, "content": "written by the fixture\n"}
	case "Read":
		input = map[string]string{"file_path": path}
	case "Bash":
		input = map[string]string{"command": "touch " + path}
	default:
		return "refused: a tool this fixture does not call"
	}
	forced := false
	if matched, err := regexp.MatchString("^(?:"+s.launch.settings.preToolMatcher+")$", tool); err == nil && matched {
		out := s.runHook("PreToolUse", s.launch.settings.preToolUse, s.hookPayload("PreToolUse", tool, input, ""))
		decision, err := preToolDecision(out)
		switch {
		case err != nil:
			return "refused: " + err.Error()
		case decision == "deny":
			return "denied"
		case decision == "ask":
			forced = true
		}
	}
	outside := tool != "Read" && !s.writable(path)
	if !forced && !outside {
		return "ran"
	}
	suggested := ""
	if outside {
		suggested = filepath.Dir(path)
	}
	out := s.runHook("PermissionRequest", s.launch.settings.permissionRequest, s.hookPayload("PermissionRequest", tool, input, suggested))
	behavior, added, removed, err := permissionDecision(out)
	if err != nil {
		return "refused: " + err.Error()
	}
	changes := ""
	for _, dir := range added {
		changes += " +" + dir
	}
	for _, dir := range removed {
		changes += " -" + dir
	}
	switch behavior {
	case "allow":
		s.mu.Lock()
		for _, dir := range added {
			if !slices.Contains(s.added, dir) {
				s.added = append(s.added, dir)
			}
		}
		s.added = slices.DeleteFunc(s.added, func(dir string) bool { return slices.Contains(removed, dir) })
		s.mu.Unlock()
		return "ran" + changes
	case "deny":
		return "denied" + changes
	}
	return "prompted"
}

// writable says whether a path lies in one of the session's working
// directories.
func (s *claudeSession) writable(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, dir := range append([]string{workingDirectory()}, s.added...) {
		if rel, err := filepath.Rel(dir, path); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
			return true
		}
	}
	return false
}

// hookPayload is what the harness hands a permission hook, in the shape seen
// in the probe; suggested, when set, is the directory PermissionRequest
// proposes to add.
func (s *claudeSession) hookPayload(event, tool string, input map[string]string, suggested string) []byte {
	payload := map[string]any{
		"hook_event_name": event, "session_id": shimConversation(), "cwd": workingDirectory(),
		"transcript_path": "", "permission_mode": shimPermissionMode, "tool_name": tool, "tool_input": input,
	}
	if suggested != "" {
		payload["permission_suggestions"] = []map[string]any{{"type": "addDirectories", "directories": []string{suggested}, "destination": "session"}}
	}
	encoded, _ := json.Marshal(payload)
	return encoded
}

// preToolDecision reads a PreToolUse answer: nothing, or a decision the
// harness knows for this event.
func preToolDecision(out []byte) (string, error) {
	if strings.TrimSpace(string(out)) == "" {
		return "", nil
	}
	var answer struct {
		Specific struct {
			Event    string `json:"hookEventName"`
			Decision string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out, &answer); err != nil {
		return "", fmt.Errorf("an unreadable PreToolUse answer %q", out)
	}
	if answer.Specific.Event != "PreToolUse" || !slices.Contains([]string{"allow", "deny", "ask"}, answer.Specific.Decision) {
		return "", fmt.Errorf("a PreToolUse answer the harness does not take: %s", out)
	}
	return answer.Specific.Decision, nil
}

// permissionDecision reads a PermissionRequest answer: nothing, or allow or
// deny with the working directories it changes for the session. Any other
// update, or one for somewhere else than the session, is refused: the grant
// hook promises to write no settings.
func permissionDecision(out []byte) (behavior string, added, removed []string, err error) {
	if strings.TrimSpace(string(out)) == "" {
		return "", nil, nil, nil
	}
	var answer struct {
		Specific struct {
			Event    string `json:"hookEventName"`
			Decision struct {
				Behavior string `json:"behavior"`
				Updates  []struct {
					Kind        string   `json:"type"`
					Directories []string `json:"directories"`
					Destination string   `json:"destination"`
				} `json:"updatedPermissions"`
			} `json:"decision"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out, &answer); err != nil {
		return "", nil, nil, fmt.Errorf("an unreadable PermissionRequest answer %q", out)
	}
	decision := answer.Specific.Decision
	if answer.Specific.Event != "PermissionRequest" || (decision.Behavior != "allow" && decision.Behavior != "deny") {
		return "", nil, nil, fmt.Errorf("a PermissionRequest answer the harness does not take: %s", out)
	}
	for _, update := range decision.Updates {
		if update.Destination != "session" || len(update.Directories) == 0 {
			return "", nil, nil, fmt.Errorf("an update beyond the session: %s", out)
		}
		switch update.Kind {
		case "addDirectories":
			added = append(added, update.Directories...)
		case "removeDirectories":
			removed = append(removed, update.Directories...)
		default:
			return "", nil, nil, fmt.Errorf("an update the grant hook does not make: %s", out)
		}
	}
	return decision.Behavior, added, removed, nil
}
