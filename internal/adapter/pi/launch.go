package pi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/brief"
	"github.com/praline-labs/rewake/internal/core/role"
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
	// A worker's conversations are kept apart from main's (the owner's
	// decision, October 23, 2026), so a person's --continue or picker in a
	// checkout a worker shared never lands in the worker's.
	worker := request.Role.ID != role.Main.ID
	if worker && adapter.HasFlag(args, sessionDirFlag) {
		return adapter.LaunchPlan{}, fmt.Errorf("a pi worker's sessions are kept in rewake's %s; remove %s", workerSessions, sessionDirFlag)
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
	dir, err := agentDir()
	if err != nil {
		return adapter.LaunchPlan{}, err
	}
	if request.Intro {
		cwd, err := os.Getwd()
		if err != nil {
			return adapter.LaunchPlan{}, fmt.Errorf("cannot read the working directory: %w", err)
		}
		found := appendSystemFile(args, cwd, dir)
		args = adapter.AddFlags(args, appendFlag, brief.Intro(request.BriefContext()))
		if found != "" {
			args = adapter.AddFlags(args, appendFlag, found)
		}
	}
	// A project with its own pi resources makes pi ask whether to trust it
	// before any extension loads, rewake's included, and the run waits for a
	// person. The owner's decision of October 19, 2026: trust it by default.
	// --approve decides for this process only and saves nothing; a choice in
	// the launch's own arguments stands.
	if len(adapter.FlagValues(args, "--approve", "-a", "--no-approve", "-na")) == 0 {
		args = adapter.AddFlags(args, "--approve")
	}
	if request.Command != "" && request.Version != PinnedVersion {
		notes = append(notes, fmt.Sprintf("pi %s named with --command is not the %s this rewake was checked with", request.Version, PinnedVersion))
	}
	env := make([]string, 0, len(request.Env)+3)
	for _, value := range request.Env {
		name, _, _ := strings.Cut(value, "=")
		if name != socketEnv && name != AgentDirEnv && name != SkipVersionCheckEnv && (!worker || name != SessionDirEnv) {
			env = append(env, value)
		}
	}
	env = append(append(env, socketEnv+"="+request.Socket), isolation(dir)...)
	if worker {
		env = append(env, SessionDirEnv+"="+filepath.Join(dir, workerSessions))
	}
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
