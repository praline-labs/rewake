package container

import (
	"slices"
	"strings"
	"testing"
)

// The isolation is the arguments, so it is checked on the arguments: no
// docker is needed to see that the harness is read-only, HOME is private and
// nothing else of the host is mounted.
func TestARunMountsOnlyTheVersionAndTheOneWritableDirectory(t *testing.T) {
	args := Args(Spec{
		Version: "/cache/codex/0.156.0", Executable: "package/bin/codex",
		Args: []string{"--version"}, Writable: "/tmp/out", WritableMount: "/out",
	}, "name")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--rm", "--read-only", "--network none", "HOME=" + Home,
		"--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=", "--memory=",
		"type=bind,source=/cache/codex/0.156.0,target=" + HarnessMount + ",readonly",
		"type=bind,source=/tmp/out,target=/out",
		Image() + " " + HarnessMount + "/package/bin/codex --version",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the run lacks %q:\n%s", want, joined)
		}
	}
	mounts := 0
	for index, arg := range args {
		if arg == "--mount" || arg == "--volume" || arg == "-v" {
			mounts++
			if index+1 < len(args) && strings.Contains(args[index+1], "/home/") {
				t.Errorf("a home directory is mounted: %s", args[index+1])
			}
		}
	}
	if mounts != 2 {
		t.Errorf("%d mounts, want the version and the writable directory:\n%s", mounts, joined)
	}
	if without := Args(Spec{Version: "/v", Executable: "x"}, "n"); slices.Contains(without, "type=bind,source=,target=") {
		t.Errorf("an empty writable directory was mounted: %v", without)
	}
}

// The tag follows the recipe, so an edited recipe cannot reuse an image built
// from the old one.
func TestTheImageTagFollowsTheRecipe(t *testing.T) {
	if !strings.HasPrefix(Image(), "rewake-harness:") || len(Image()) != len("rewake-harness:")+12 {
		t.Fatalf("image tag %q", Image())
	}
}

func TestAPathWithACommaIsRefused(t *testing.T) {
	if Mountable("/tmp/a,readonly=false") == nil {
		t.Fatal("a comma in a mount path was accepted")
	}
	if _, err := Run(t.Context(), Spec{Version: "/v", Writable: "/tmp/x,y"}); err == nil || !strings.Contains(err.Error(), "comma") {
		t.Fatalf("a run with a comma in its writable path was not refused: %v", err)
	}
}
