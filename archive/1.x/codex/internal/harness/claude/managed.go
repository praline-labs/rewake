package claude

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// managedDir is the harness's managed directory, where it reads a machine's
// policy (bundled source): managed settings, and a managed-mcp.json that
// takes exclusive control of its MCP servers and makes it refuse every
// --mcp-config, ours included (live, 2.1.284, October 4, 2026). A variable so
// a test can point it elsewhere.
var managedDir = func() string {
	if runtime.GOOS == "darwin" {
		return "/Library/Application Support/ClaudeCode"
	}
	return "/etc/claude-code"
}()

// managedMCP is why a managed MCP file leaves the tool out, "" when it is
// proven absent. Only its existence is asked, by lstat: present in any form —
// a link, a directory, unreadable — the harness takes it as in control, and
// only ENOENT proves it absent; anything else could not be checked and leaves
// the tool out too (docs/mail-bridge-launch.md#claude-code). It never refuses
// the launch.
func managedMCP() string {
	_, err := os.Lstat(filepath.Join(managedDir, "managed-mcp.json"))
	switch {
	case err == nil:
		return "a managed MCP configuration is present"
	case errors.Is(err, fs.ErrNotExist):
		return ""
	}
	return "the managed MCP configuration could not be checked"
}
