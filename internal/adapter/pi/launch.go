package pi

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/brief"
)

func (harness) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	if request.RunDir == "" || request.Socket == "" {
		return adapter.LaunchPlan{}, fmt.Errorf("pi needs a run directory and socket for its extension")
	}
	if len(request.Socket) > 107 {
		return adapter.LaunchPlan{}, fmt.Errorf("pi's socket path exceeds 107 bytes; use a shorter session name or REWAKE_DIR")
	}
	args := append([]string{}, request.Args...)
	if adapter.HasFlag(args, worktreeFlag) {
		return adapter.LaunchPlan{}, fmt.Errorf("%s reached pi's launch; start through rewake pi %s so rewake makes the checkout", worktreeFlag, worktreeFlag)
	}
	if request.Conversation != "" {
		if flag := continued(args); flag != "" {
			return adapter.LaunchPlan{}, fmt.Errorf("rewake names this launch's new conversation and %s names another; remove it", flag)
		}
		if adapter.HasFlag(args, "--no-session") {
			return adapter.LaunchPlan{}, fmt.Errorf("rewake names this launch's conversation; remove --no-session")
		}
		args = adapter.AddFlags(args, sessionIDFlag, request.Conversation)
	}
	args, notes := adapter.ApplyDefaults(args, defaults())
	file := filepath.Join(request.RunDir, "extension.ts")
	// -e adds rewake's extension to those pi finds, the person's and the
	// project's, which load as they would without rewake.
	args = adapter.AddFlags(args, "-e", file)
	if request.Intro {
		args = adapter.AddFlags(args, "--append-system-prompt", brief.Intro(request.BriefContext()))
	}
	dir, err := agentDir()
	if err != nil {
		return adapter.LaunchPlan{}, err
	}
	if request.Command != "" && request.Version != PinnedVersion {
		notes = append(notes, fmt.Sprintf("pi %s named with --command is not the %s this rewake was checked with", request.Version, PinnedVersion))
	}
	env := make([]string, 0, len(request.Env)+3)
	for _, value := range request.Env {
		name, _, _ := strings.Cut(value, "=")
		if name != socketEnv && name != agentDirEnv && name != skipVersionCheckEnv {
			env = append(env, value)
		}
	}
	env = append(env, socketEnv+"="+request.Socket, agentDirEnv+"="+dir, skipVersionCheckEnv+"=1")
	run := newRun(file, request.Socket)
	run.epoch, run.controlDir = request.Epoch, request.ControlDir
	command, args := request.Invocation(ID, args)
	return adapter.LaunchPlan{
		Command: command, Args: args, Env: env, Notes: notes,
		Socket: request.Socket, OwnsSocket: true,
		Service: run, Wake: run, TurnBoundary: run, Thread: run, Live: run, Telemetry: run,
	}, nil
}

func defaults() []adapter.Default {
	return []adapter.Default{
		{Env: "REWAKE_PI_MODEL", What: "model", Present: func(args []string) bool {
			return adapter.HasFlag(args, "--model") || adapter.HasFlag(args, "--models")
		}, Apply: func(args []string, value string) []string { return adapter.AddFlags(args, "--model", value) }},
		{Env: "REWAKE_PI_EFFORT", What: "reasoning effort", Present: thinkingNamed, Apply: func(args []string, value string) []string { return adapter.AddFlags(args, "--thinking", value) }},
	}
}

func continued(args []string) string {
	for _, flag := range []string{sessionIDFlag, "--session", "--continue", "-c", "--resume", "-r", "--fork"} {
		if adapter.HasFlag(args, flag) {
			return flag
		}
	}
	return ""
}

// The native model selector accepts a thinking suffix. A default must not
// override that explicit choice either.
func thinkingNamed(args []string) bool {
	if adapter.HasFlag(args, "--thinking") {
		return true
	}
	for _, value := range adapter.FlagValues(args, "--model", "--models") {
		for _, model := range strings.Split(value, ",") {
			colon := strings.LastIndexByte(model, ':')
			if colon < 0 {
				continue
			}
			switch model[colon+1:] {
			case "off", "minimal", "low", "medium", "high", "xhigh", "max":
				return true
			}
		}
	}
	return false
}
