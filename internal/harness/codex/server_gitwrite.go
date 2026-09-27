package codex

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

type threadStatus struct {
	Kind string `json:"type"`
}

type threadEnvironment struct {
	ID    string   `json:"environmentId"`
	Cwd   string   `json:"cwd"`
	Roots []string `json:"runtimeWorkspaceRoots"`
}

// threadRead is one read of the thread's roots and status, taken once per
// notice. Roots are a replacement field, not a grant list: neither the launch
// cwd nor a cached resume reply can preserve roots changed by the person in
// the TUI, so each notice that changes them reads a fresh snapshot.
type threadRead struct {
	id           string
	status       string
	environments []threadEnvironment
	err          error
}

func readThreadRoots(ctx context.Context, readThread func(context.Context, any) error) threadRead {
	var response struct {
		Thread struct {
			ID           string              `json:"id"`
			Status       threadStatus        `json:"status"`
			Environments []threadEnvironment `json:"environments"`
		} `json:"thread"`
	}
	readCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if err := readThread(readCtx, &response); err != nil {
		return threadRead{err: err}
	}
	return threadRead{id: response.Thread.ID, status: response.Thread.Status.Kind, environments: response.Thread.Environments}
}

// local returns the thread's one local environment, or why its roots cannot
// be changed. Top-level roots rebuild the default environment selection: do
// not project multiple or remote environments into one local list and discard
// their state.
func (read threadRead) local(thread string) (threadEnvironment, string) {
	if read.err != nil {
		return threadEnvironment{}, "could not read current thread roots"
	}
	if read.id != thread || len(read.environments) != 1 || read.environments[0].ID != "local" {
		return threadEnvironment{}, "current local workspace roots are unavailable"
	}
	environment := read.environments[0]
	if environment.Roots == nil || !localRoot(environment.Cwd) || !allLocalRoots(environment.Roots) {
		return threadEnvironment{}, "current local workspace roots are unavailable"
	}
	return environment, ""
}

// addGitRoots adds the metadata of the thread's own checkout, for an explicit
// --grant-git, and says what it did. Failure must not prevent ordinary
// delivery: the task goes without the grant, and the detail says so.
func addGitRoots(roots *[]string, environment threadEnvironment, active bool) string {
	metadata, err := gitMetadataDirectories(environment.Cwd)
	if err != nil {
		return "Git metadata access unchanged: could not resolve the thread's Git metadata"
	}
	if len(addRoots(roots, metadata)) == 0 {
		return ""
	}
	if active {
		return "Git metadata roots added for subsequent turns; the active turn keeps its existing permissions"
	}
	return "Git metadata roots added; the selected permission policy still applies"
}

// addRoots appends each directory no root names already, and returns those.
func addRoots(roots *[]string, directories []string) []string {
	var added []string
	for _, directory := range directories {
		if !hasRoot(*roots, directory) {
			*roots = append(*roots, directory)
			added = append(added, directory)
		}
	}
	return added
}

func hasRoot(roots []string, directory string) bool {
	for _, root := range roots {
		if filepath.Clean(root) == directory {
			return true
		}
	}
	return false
}

func localRoot(path string) bool {
	return filepath.IsAbs(path) && !strings.ContainsRune(path, '\x00')
}

func allLocalRoots(roots []string) bool {
	for _, root := range roots {
		if !localRoot(root) {
			return false
		}
	}
	return true
}
