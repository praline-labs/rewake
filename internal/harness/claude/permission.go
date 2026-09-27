package claude

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/grant"
)

// A directory granted with a task reaches Claude Code through its permission
// hooks: nothing else adds a working directory to a running session. Seen live
// on 2.1.280 (docs/research-claude-actions.md#a-directory-given-to-a-running-session):
//
//   - a PermissionRequest hook that answers allow with updatedPermissions
//     addDirectories, destination session, makes the directory a working one
//     until the session ends, and the model is told so; nothing is written to
//     disk;
//   - the same answer with removeDirectories takes one out, on any request,
//     a directory given with --add-dir included;
//   - PreToolUse cannot change permissions, but its answer ask makes the
//     harness raise a PermissionRequest for a call it would have run anyway.
//
// So a grant is given when the session first asks to write in it, and taken
// back at the first tool call after its task is settled. The hook answers
// only what it can prove from the journal and is silent otherwise: silence
// leaves the call to the person and the permission mode.

// Hook events the grant hook answers.
const (
	preToolUse        = "PreToolUse"
	permissionRequest = "PermissionRequest"
)

// grantHookInput is the part of a hook's payload the grant hook reads.
type grantHookInput struct {
	Event       string                     `json:"hook_event_name"`
	Tool        string                     `json:"tool_name"`
	Input       map[string]json.RawMessage `json:"tool_input"`
	Suggestions []permissionUpdate         `json:"permission_suggestions"`
	CWD         string                     `json:"cwd"`
	Mode        string                     `json:"permission_mode"`
}

// permissionUpdate is one entry of permission_suggestions, and of the
// updatedPermissions an answer carries.
type permissionUpdate struct {
	Kind        string   `json:"type"`
	Directories []string `json:"directories,omitempty"`
	Destination string   `json:"destination,omitempty"`
}

// GrantAnswer is what the grant hook does with one call: what it prints, and
// which directories that answer takes out of the session, for the journal.
type GrantAnswer struct {
	Output  []byte
	Removed []string
}

// fileWriters name the path their call writes; notebookPath is NotebookEdit's.
var fileWriters = map[string]string{"Write": "file_path", "Edit": "file_path", "MultiEdit": "file_path", "NotebookEdit": "notebook_path"}

// readers are the tools that only read, with the field naming where. A path
// left out means the working directory.
var readers = map[string]string{"Read": "file_path", "Glob": "path", "Grep": "path", "LS": "path", "NotebookRead": "notebook_path"}

// forcingModes are where a read in the working directory runs without asking,
// so an ask forced on it approves nothing the mode would not. bypassPermissions
// asks nothing and might show a person the forced question; plan and the rest
// are not known to behave so.
var forcingModes = []string{"default", "acceptEdits"}

// DecideGrant answers one PreToolUse or PermissionRequest of a session from
// its journal. It never fails: a payload it cannot read is a call it is
// silent about.
func DecideGrant(payload []byte, entries []grant.Entry) GrantAnswer {
	var in grantHookInput
	if json.Unmarshal(payload, &in) != nil {
		return GrantAnswer{}
	}
	var live, revoking []string
	for _, entry := range entries {
		switch {
		case entry.Live():
			live = append(live, entry.Path)
		case entry.Revoking() && !slices.Contains(revoking, entry.Path):
			revoking = append(revoking, entry.Path)
		}
	}
	// A directory another live task holds is not taken back.
	revoking = slices.DeleteFunc(revoking, func(path string) bool { return slices.Contains(live, path) })
	if len(live) == 0 && len(revoking) == 0 {
		return GrantAnswer{}
	}
	switch in.Event {
	case preToolUse:
		return in.preToolUse(revoking)
	case permissionRequest:
		return in.permissionRequest(live, revoking)
	}
	return GrantAnswer{}
}

// preToolUse denies a file tool writing where a grant is being taken back,
// and forces a question on a read that would run anyway, so the answer to it
// can take the directory out.
func (in grantHookInput) preToolUse(revoking []string) GrantAnswer {
	if len(revoking) == 0 {
		return GrantAnswer{}
	}
	if field, ok := fileWriters[in.Tool]; ok {
		path := in.path(field)
		for _, root := range revoking {
			if path != "" && grant.Within(path, root) {
				return hookAnswer(map[string]any{
					"hookEventName":            preToolUse,
					"permissionDecision":       "deny",
					"permissionDecisionReason": "rewake: write access to " + root + " was taken back, its task reported on; ask main for a new grant if the work needs it",
				}, nil)
			}
		}
		return GrantAnswer{}
	}
	if !in.forcible() {
		return GrantAnswer{}
	}
	return hookAnswer(map[string]any{
		"hookEventName":            preToolUse,
		"permissionDecision":       "ask",
		"permissionDecisionReason": "rewake: taking back write access to " + strings.Join(revoking, ", "),
	}, nil)
}

// permissionRequest allows a write inside a live grant, adding its root, and
// takes out what is being taken back when it answers at all.
func (in grantHookInput) permissionRequest(live, revoking []string) GrantAnswer {
	var updates []permissionUpdate
	if roots := in.grantedRoots(live); roots != nil {
		updates = append(updates, permissionUpdate{Kind: "addDirectories", Directories: roots, Destination: "session"})
	} else if len(revoking) == 0 || !in.forcible() {
		// Not a write rewake granted, and not the question it forced: the
		// person or the mode decides.
		return GrantAnswer{}
	}
	if len(revoking) > 0 {
		updates = append(updates, permissionUpdate{Kind: "removeDirectories", Directories: revoking, Destination: "session"})
	}
	return hookAnswer(map[string]any{
		"hookEventName": permissionRequest,
		"decision":      map[string]any{"behavior": "allow", "updatedPermissions": updates},
	}, revoking)
}

// grantedRoots are the live grants a call writes in, or nil when it writes
// anywhere else too. A file tool is judged by its path, which is all it
// writes. Any other tool is judged by what the harness suggests adding: the
// call is allowed only when every suggestion is a directory inside a grant,
// that is, when the grant alone would have let it run — an approved command
// runs whole, so one that needs anything more is left to the person.
func (in grantHookInput) grantedRoots(live []string) []string {
	var paths []string
	if field, ok := fileWriters[in.Tool]; ok {
		path := in.path(field)
		if path == "" {
			return nil
		}
		paths = []string{path}
	} else {
		if len(in.Suggestions) == 0 {
			return nil
		}
		for _, suggestion := range in.Suggestions {
			if suggestion.Kind != "addDirectories" || len(suggestion.Directories) == 0 {
				return nil
			}
			for _, directory := range suggestion.Directories {
				paths = append(paths, in.resolved(directory))
			}
		}
	}
	var roots []string
	for _, path := range paths {
		at := slices.IndexFunc(live, func(root string) bool { return grant.Covers(root, path) })
		if at < 0 {
			return nil
		}
		if !slices.Contains(roots, live[at]) {
			roots = append(roots, live[at])
		}
	}
	return roots
}

// forcible says whether the call reads inside the working directory in a mode
// that runs such a read without asking.
func (in grantHookInput) forcible() bool {
	field, ok := readers[in.Tool]
	if !ok || !slices.Contains(forcingModes, in.Mode) || in.CWD == "" {
		return false
	}
	path := in.path(field)
	if path == "" {
		path = in.resolved(in.CWD)
	}
	return grant.Within(path, in.resolved(in.CWD))
}

// path is a tool's path argument, absolute and resolved as far as it exists,
// or "" when the call names none.
func (in grantHookInput) path(field string) string {
	var given string
	if raw, ok := in.Input[field]; !ok || json.Unmarshal(raw, &given) != nil || given == "" {
		return ""
	}
	return in.resolved(given)
}

func (in grantHookInput) resolved(given string) string {
	if !filepath.IsAbs(given) {
		if in.CWD == "" {
			return ""
		}
		given = filepath.Join(in.CWD, given)
	}
	return grant.ResolveExisting(given)
}

func hookAnswer(specific map[string]any, removed []string) GrantAnswer {
	encoded, err := json.Marshal(map[string]any{"hookSpecificOutput": specific})
	if err != nil {
		return GrantAnswer{}
	}
	return GrantAnswer{Output: encoded, Removed: removed}
}
