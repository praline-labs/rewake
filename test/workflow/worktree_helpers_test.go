package workflow

import (
	"encoding/json"
	"os"
)

// What the worktree scenarios read: the directory each process of a harness
// started in, and rewake's own account of its worktrees.

// registeredCWD is the directory a session registered as its own, or "".
func registeredCWD(rewake func(args ...string) (string, error), name string) string {
	out, err := rewake("list", "--json")
	if err != nil {
		return ""
	}
	var listing struct {
		Sessions []struct {
			Name string `json:"name"`
			CWD  string `json:"cwd"`
		} `json:"sessions"`
	}
	if json.Unmarshal([]byte(out), &listing) != nil {
		return ""
	}
	for _, session := range listing.Sessions {
		if session.Name == name {
			return session.CWD
		}
	}
	return ""
}

// shimCwdFile makes each process of a harness program write the directory it
// was started in to this path, with the name of its half appended.
const shimCwdFile = "RW_SHIM_CWD_FILE"

func recordShimCwd(half string) {
	target := os.Getenv(shimCwdFile)
	if target == "" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "unreadable: " + err.Error()
	}
	_ = os.WriteFile(target+"."+half, []byte(cwd), 0o600)
}

// treeView is the part of `rewake worktree ls --json` the scenario reads.
type treeView struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Commit  string `json:"commit"`
	Running bool   `json:"running"`
	Session *struct {
		Name string `json:"name"`
	} `json:"session"`
}

// listTreesFrom is `rewake worktree ls --json` read through a runner.
func listTreesFrom(rewake func(args ...string) (string, error)) []treeView {
	out, err := rewake("worktree", "ls", "--json")
	var listing struct {
		Worktrees []treeView `json:"worktrees"`
	}
	if err != nil || json.Unmarshal([]byte(out), &listing) != nil {
		return nil
	}
	return listing.Worktrees
}
