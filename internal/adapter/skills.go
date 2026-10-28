package adapter

import (
	"os"
	"path/filepath"
)

// SkillCatalog is where a harness run as main reads its skills: the folders
// in the order the harness itself looks, each holding a skill as a folder
// <name>/SKILL.md, the first that has the name giving it. A brief's skills
// are copied from there (docs/briefs.md#skills); a harness that declares none
// gives a brief no skills. getenv is main's environment, home its user's home
// and dir the directory main runs in.
type SkillCatalog interface {
	SkillFolders(getenv func(string) string, home, dir string) []string
}

// UpToRepository is dir and its parents, nearest first, up to the root of the
// git repository that holds dir — a directory with a .git entry, a worktree's
// file included — and never stop or above it; without a repository, every
// parent below stop. An empty stop stops nowhere.
func UpToRepository(dir, stop string) []string {
	var dirs []string
	for at := filepath.Clean(dir); ; {
		if stop != "" && at == filepath.Clean(stop) {
			return dirs
		}
		dirs = append(dirs, at)
		if _, err := os.Lstat(filepath.Join(at, ".git")); err == nil {
			return dirs
		}
		parent := filepath.Dir(at)
		if parent == at {
			return dirs
		}
		at = parent
	}
}
