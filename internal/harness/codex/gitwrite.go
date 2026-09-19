package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// The state directory in LaunchRequest is not the working directory. A caller
// may also change the latter with a harness flag, including its joined forms.
func gitWorkingDirectory(args []string) (string, error) {
	base, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot resolve the working directory: %w", err)
	}
	selected := base
	visible := harness.BeforeTerminator(args)
	for index := 0; index < len(visible); index++ {
		arg := visible[index]
		switch {
		case arg == "-C" || arg == "--cd":
			index++
			if index == len(visible) {
				return "", fmt.Errorf("%s has no working directory", arg)
			}
			selected = visible[index]
		case strings.HasPrefix(arg, "--cd="):
			selected = strings.TrimPrefix(arg, "--cd=")
		case strings.HasPrefix(arg, "-C") && len(arg) > 2:
			selected = strings.TrimPrefix(arg[2:], "=")
		}
	}
	if selected == "" {
		return "", fmt.Errorf("the working directory is empty")
	}
	if !filepath.IsAbs(selected) {
		selected = filepath.Join(base, selected)
	}
	resolved, err := filepath.EvalSymlinks(selected)
	if err != nil {
		return "", fmt.Errorf("cannot resolve working directory %s: %w", selected, err)
	}
	return resolved, nil
}
