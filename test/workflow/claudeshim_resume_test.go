package workflow

// A resumed launch of this column's session.
//
// The harness resumes a conversation with --resume <id>, and every hook and
// status line after it names that id as session_id. A cold resume does not
// bring back a directory a hook added for the session, and --add-dir beside
// --resume does; a hook's removeDirectories takes out a directory given at
// launch as well as one it added (seen live on 2.1.280,
// docs/research-claude-actions.md). So the fixture starts with the launch's
// directories as working directories a hook may take out, and records what it
// was launched with for a case to read.

import (
	"slices"
	"strings"
)

// resume takes up what a launch says of the conversation: its id, and the
// working directories it was given.
func (s *claudeSession) resume() {
	if s.launch.resume != "" {
		shimConversationState.Lock()
		shimConversationState.resumed = s.launch.resume
		shimConversationState.Unlock()
	}
	s.mu.Lock()
	for _, dir := range s.launch.addDirs {
		if !slices.Contains(s.added, dir) {
			s.added = append(s.added, dir)
		}
	}
	s.mu.Unlock()
	if len(s.launch.addDirs) > 0 || s.launch.resume != "" {
		(&shimSession{}).recordTurn("launched resume=" + s.launch.resume + " add-dir=" + strings.Join(s.launch.addDirs, ","))
	}
}

// startSource is what SessionStart says of how the session began: "resume"
// for a resumed conversation, as the harness says, "startup" otherwise.
func (s *claudeSession) startSource() string {
	if s.launch.resume != "" {
		return "resume"
	}
	return "startup"
}
