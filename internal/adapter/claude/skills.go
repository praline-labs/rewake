package claude

import (
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
)

// SkillFolders are the folders Claude Code reads its own skills from, in its
// order (2.1.310, docs/research-launch.md#where-claude-code-reads-skills): the
// user's, in its configuration directory, then .claude/skills in the
// directory it runs in and each parent up to the repository's root, never
// the home directory itself. Managed skills, plugins' skills and --add-dir
// folders are not offered to a brief.
func (claudeHarness) SkillFolders(getenv func(string) string, home, dir string) []string {
	config := getenv(configDirEnv)
	if config == "" {
		config = filepath.Join(home, ".claude")
	}
	folders := []string{filepath.Join(config, "skills")}
	for _, at := range adapter.UpToRepository(dir, home) {
		folders = append(folders, filepath.Join(at, ".claude", "skills"))
	}
	return folders
}
