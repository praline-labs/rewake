package pi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The worktree and brief tools' contract with pi, and the face's mirror: argv, directory, bound,
// and every failure shown as an error (worktree.test.ts, briefs.test.ts).
func TestTheWorktreeToolKeepsItsContract(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, bun, "test", "./worktree.test.ts", "./management.test.ts", "./briefs.test.ts", "./mirror.test.ts")
	command.Env = withoutRewakeEnv()
	out, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(out), " 0 fail") {
		t.Fatalf("the tool's tests: %v\n%s", err, out)
	}
}

// withoutRewakeEnv is this process's environment less every variable of
// rewake's: a session running the tests exports its own, and the tool reads
// them as its defaults — REWAKE_ROLE=main made it offer main's tools too.
func withoutRewakeEnv() []string {
	return slices.DeleteFunc(os.Environ(), func(entry string) bool {
		return strings.HasPrefix(entry, "REWAKE_")
	})
}

// A run writes every module of the extension beside it: one it misses fails
// the extension's import, and rewake's mail with it.
func TestARunWritesEveryModuleOfTheExtension(t *testing.T) {
	rig := newRig(t, nil)
	modules, err := filepath.Glob("*.ts")
	if err != nil || len(modules) == 0 {
		t.Fatalf("the extension's modules: %v %v", modules, err)
	}
	for _, module := range modules {
		if strings.HasSuffix(module, ".test.ts") {
			continue
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(rig.run.file), module)); err != nil {
			t.Errorf("the run lacks %s: %v", module, err)
		}
	}
}
