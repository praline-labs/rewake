package claude

import (
	"os"
	"path/filepath"
)

// ProtectedDirs: Claude Code keeps its configuration in CLAUDE_CONFIG_DIR when
// set, and otherwise in ~/.claude and ~/.claude.json, beside which other
// launchers keep their own ~/.claude-<name>; its installed versions are in
// ~/.local/share/claude.
func (claudeHarness) ProtectedDirs() []string {
	var dirs []string
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		dirs = append(dirs, dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return dirs
	}
	dirs = append(dirs, filepath.Join(home, ".claude"), filepath.Join(home, ".local", "share", "claude"))
	named, _ := filepath.Glob(filepath.Join(home, ".claude*"))
	return append(dirs, named...)
}
