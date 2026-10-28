package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/host/launch"
)

// skillCatalog finds a brief's skills where main's harness reads its own
// (docs/briefs.md#skills): main is the caller, so its harness, its directory
// and its environment are this process's. A skill a [skills] never list
// names is refused, naming the file. It is asked only when the text names a
// skill, so a brief without one needs neither the harness nor the files.
func skillCatalog(ctx *Context, caller briefRun) taskbrief.Catalog {
	return func(name string) (string, error) {
		harness, found := ctx.table.catalog.Find(caller.session.Harness)
		catalog, offers := harness.(adapter.SkillCatalog)
		if !found || !offers {
			return "", fmt.Errorf("main's harness %q reads no skill folders rewake knows, so a brief of it gives none", caller.session.Harness)
		}
		set := launch.LoadIn(caller.session.CWD)
		if err := set.Err(); err != nil {
			return "", fmt.Errorf("the alias files do not read, so which skills are barred is not known: %w", err)
		}
		if file := set.SkillBarred(name); file != "" {
			return "", fmt.Errorf("%s is barred from briefs by [skills] never in %s", name, file)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		folders := catalog.SkillFolders(os.Getenv, home, caller.session.CWD)
		for _, folder := range folders {
			at := filepath.Join(folder, name)
			if info, err := os.Stat(filepath.Join(at, "SKILL.md")); err == nil && info.Mode().IsRegular() {
				return at, nil
			}
		}
		return "", fmt.Errorf("no skill %s: none of %s holds %s/SKILL.md", name, strings.Join(folders, ", "), name)
	}
}
