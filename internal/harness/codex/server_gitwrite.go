package codex

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

type threadEnvironment struct {
	ID    string   `json:"environmentId"`
	Cwd   string   `json:"cwd"`
	Roots []string `json:"runtimeWorkspaceRoots"`
}

// Roots are a replacement field, not a grant list. Read a fresh server snapshot
// for each task; neither launch cwd nor a cached resume reply can preserve roots
// changed by the person in the TUI. Failure must not prevent ordinary delivery.
func (s *serverSession) taskGitRoots(ctx context.Context, client *rpcClient, thread string, kind inbox.Kind) ([]string, string) {
	if !s.gitWrite || kind != inbox.Task && kind != inbox.Question {
		return nil, ""
	}
	var response struct {
		Thread struct {
			ID           string              `json:"id"`
			Status       threadStatus        `json:"status"`
			Environments []threadEnvironment `json:"environments"`
		} `json:"thread"`
	}
	readCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if err := client.call(readCtx, "thread/read", map[string]any{"threadId": thread, "includeTurns": false}, &response); err != nil {
		return nil, "Git metadata access unchanged: could not read current thread roots"
	}
	current := response.Thread
	// Top-level roots rebuild the default environment selection. Do not project
	// multiple or remote environments into one local list and discard their state.
	if current.ID != thread || len(current.Environments) != 1 || current.Environments[0].ID != "local" {
		return nil, "Git metadata access unchanged: current local workspace roots are unavailable"
	}
	environment := current.Environments[0]
	if environment.Roots == nil || !localRoot(environment.Cwd) || !allLocalRoots(environment.Roots) {
		return nil, "Git metadata access unchanged: current local workspace roots are unavailable"
	}
	metadata, err := gitMetadataDirectories(environment.Cwd)
	if err != nil {
		return nil, "Git metadata access unchanged: could not resolve the thread's Git metadata"
	}
	roots := slices.Clone(environment.Roots)
	for _, directory := range metadata {
		if !slices.ContainsFunc(roots, func(root string) bool { return filepath.Clean(root) == directory }) {
			roots = append(roots, directory)
		}
	}
	if len(roots) == len(environment.Roots) {
		return nil, ""
	}
	if current.Status.Kind == "active" {
		return roots, "Git metadata roots added for subsequent turns; the active turn keeps its existing permissions"
	}
	return roots, "Git metadata roots added; the selected permission policy still applies"
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
