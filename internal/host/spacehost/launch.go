package spacehost

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// launcherBound is how long rewake space open waits for the launcher, which
// only starts the space host and exits.
const launcherBound = 10 * time.Second

// Start runs the launcher for spec and waits for it to exit: once it has, the
// space host runs on its own, reparented away from the caller. The launcher
// gets a session of its own, so nothing of main's terminal reaches it or
// what it starts. The space host's progress is read from the record.
func Start(spec Spec) error {
	spec.Stage = stageLaunch
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find this rewake to run the space host: %w", err)
	}
	env, err := withSpec(hostEnv(os.Environ(), spec), spec)
	if err != nil {
		return err
	}
	cmd := exec.Command(self)
	cmd.Env, cmd.Dir = env, "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start the space host's launcher: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("the space host's launcher failed: %w", err)
		}
		return nil
	case <-time.After(launcherBound):
		_ = cmd.Process.Kill()
		return fmt.Errorf("the space host's launcher did not exit within %s", launcherBound)
	}
}

// launch is the launcher's stage: start the space host and exit without
// waiting for it.
func launch(spec Spec) int {
	spec.Stage, spec.Launcher = stageServe, os.Getpid()
	self, err := os.Executable()
	if err != nil {
		return 1
	}
	env, err := withSpec(os.Environ(), spec)
	if err != nil {
		return 1
	}
	cmd := exec.Command(self)
	cmd.Env, cmd.Dir = env, "/"
	if err := cmd.Start(); err != nil {
		return 1
	}
	_ = cmd.Process.Release()
	return 0
}

// hostEnv is the environment of the launcher and the space host: the
// caller's, as no session's, in main's room. The worker's is made from it by
// WorkerEnv.
func hostEnv(env []string, spec Spec) []string {
	out := make([]string, 0, len(env)+2)
	for _, kv := range env {
		switch name, _, _ := strings.Cut(kv, "="); name {
		case state.SessionEnv, state.EpochEnv, state.DirEnv, state.RoomEnv:
			continue
		}
		out = append(out, kv)
	}
	return append(out, state.DirEnv+"="+spec.Base, state.RoomEnv+"="+spec.Room)
}
