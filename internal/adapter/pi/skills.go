package pi

import (
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
)

// SkillFolders are the folders pi reads its own skills from, in the order it
// finds them, the first of a name kept (1.4.0, docs/research-pi.md#skills):
// the project's .pi/skills, .agents/skills in the directory it runs in and
// each parent up to the repository's root, the agent directory's skills —
// rewake's own for a pi rewake starts — and the user's ~/.agents/skills. pi
// also takes a name from SKILL.md's front matter and finds skills nested
// deeper; a brief names a skill by its folder.
func (harness) SkillFolders(getenv func(string) string, home, dir string) []string {
	folders := []string{filepath.Join(dir, ".pi", "skills")}
	user := filepath.Join(home, ".agents", "skills")
	for _, at := range adapter.UpToRepository(dir, "") {
		if folder := filepath.Join(at, ".agents", "skills"); folder != user {
			folders = append(folders, folder)
		}
	}
	agent := getenv(AgentDirEnv)
	if agent == "" {
		agent = filepath.Join(home, ".pi", "agent")
	}
	return append(folders, filepath.Join(agent, "skills"), user)
}
