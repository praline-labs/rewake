package harness

// What a settings file must not be able to do. These came from the acceptance
// review: a file that is a named pipe hung the launch for ever at open, and a
// value was cut at a '#' that belonged to it.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSettingsComments(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`REWAKE_CODEX_EFFORT= # disabled`, ""},
		{`REWAKE_CODEX_MODEL="name # suffix"`, "name # suffix"},
	} {
		_, got, note := parseSetting(tc.raw)
		if got != tc.want || note != "" {
			t.Errorf("%q: got %q note %q, want %q", tc.raw, got, note, tc.want)
		}
	}
}

func TestSettingsFIFO(t *testing.T) {
	if os.Getenv("REWAKE_REVIEW_FIFO_CHILD") == "1" {
		LoadSettings()
		return
	}
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, ".rewake.env"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Generous on purpose: a blocked open never returns, so any limit tells
	// the two apart, while a second is not always enough for a race-enabled
	// test binary to start at all.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSettingsFIFO$", "-test.timeout=25s")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+dir, "REWAKE_REVIEW_FIFO_CHILD=1")
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatal("LoadSettings blocked opening a FIFO without a writer")
	}
	if err != nil {
		t.Fatal(err)
	}
}
