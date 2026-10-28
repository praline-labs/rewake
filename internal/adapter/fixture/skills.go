//go:build rewaketest

package fixture

import "path/filepath"

// SkillFolders gives the fixture, run as main, one folder of skills in its
// home directory, so a workflow scenario can make a brief that names one.
func (fixtureHarness) SkillFolders(_ func(string) string, home, _ string) []string {
	return []string{filepath.Join(home, ".fixture", "skills")}
}
