package codex

import (
	"strings"

	"github.com/praline-labs/rewake/internal/harness"
)

// Remote continuation rejects permission overrides before contacting the server.
// Inspect option values separately so a model, path or prompt named resume does
// not suppress metadata access on a fresh launch. Leave validation to the TUI.
func continuationOptions(args []string) (bool, bool) {
	command, permissions := continuationMode(args)
	return command == "resume" || command == "fork", permissions
}

func continuationMode(args []string) (command string, permissionOverride bool) {
	visible := harness.BeforeTerminator(args)
	positional := false
	for i := 0; i < len(visible); i++ {
		arg := visible[i]
		if !strings.HasPrefix(arg, "-") {
			if !positional {
				command = arg
				positional = true
			}
			continue
		}
		name, value, joined := strings.Cut(arg, "=")
		if !strings.HasPrefix(arg, "--") && len(arg) > 2 && strings.ContainsRune("cCmpisa", rune(arg[1])) {
			name, value, joined = arg[:2], strings.TrimPrefix(arg[2:], "="), true
		}
		switch name {
		case "--add-dir", "--sandbox", "-s", "--ask-for-approval", "-a",
			"--approve-for-me", "--not-so-yolo", "--dangerously-bypass-approvals-and-sandbox", "--yolo":
			permissionOverride = true
		}
		switch name {
		case "-c", "--config", "-C", "--cd", "-m", "--model", "-p", "--profile",
			"--add-dir", "-s", "--sandbox", "-a", "--ask-for-approval",
			"--remote", "--local-provider", "--enable", "--disable":
			if !joined && i+1 < len(visible) {
				i++
				value = visible[i]
			}
			if name == "-c" || name == "--config" {
				permissionOverride = permissionOverride || permissionConfig(value)
			}
		case "-i", "--image":
			// Images are variadic; their values are not subcommands or flags.
			for i+1 < len(visible) && !strings.HasPrefix(visible[i+1], "-") {
				i++
			}
		}
	}
	return command, permissionOverride
}

// Match the session-layer keys inspected by the upstream remote startup guard,
// including dotted paths and TOML's quoted spelling of a table name.
func permissionConfig(value string) bool {
	key, _, _ := strings.Cut(value, "=")
	root, _, _ := strings.Cut(strings.TrimSpace(key), ".")
	switch strings.Trim(strings.TrimSpace(root), "\"'") {
	case "approval_policy", "approvals_reviewer", "sandbox_mode", "default_permissions",
		"permissions", "network", "sandbox_workspace_write":
		return true
	}
	return false
}
