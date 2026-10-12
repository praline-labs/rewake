package endpoint

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// peerHelperEnv makes the test binary wait on its stdin instead of testing:
// a peer to check that runs some image.
const peerHelperEnv = "REWAKE_ENDPOINT_PEER_HELPER"

func TestPeerHelper(t *testing.T) {
	if os.Getenv(peerHelperEnv) == "" {
		t.Skip("only a peer started by TestPeerCheckIsTheBuildsContent runs this")
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}

// startPeer runs an image as a peer, alive until the test ends.
func startPeer(t *testing.T, image string) int {
	t.Helper()
	cmd := exec.Command(image, "-test.run=^TestPeerHelper$")
	cmd.Env = append(os.Environ(), peerHelperEnv+"=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Wait() })
	return cmd.Process.Pid
}

// The peer check compares the content of what the peer runs, the identity a
// room's lease compares: a byte-identical copy is this build, an image one
// byte longer is not, however alike their versions.
func TestPeerCheckIsTheBuildsContent(t *testing.T) {
	data, err := os.ReadFile("/proc/self/exe")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	copied, changed := filepath.Join(dir, "copy"), filepath.Join(dir, "changed")
	if err := os.WriteFile(copied, data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, append(data, 0), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := sameBuild(os.Getpid()); err != nil {
		t.Fatalf("this process: %v", err)
	}
	if err := sameBuild(startPeer(t, copied)); err != nil {
		t.Fatalf("a byte-identical copy: %v", err)
	}
	if err := sameBuild(startPeer(t, changed)); err == nil || err.Error() != "its executable is another build" {
		t.Fatalf("an image one byte longer: %v", err)
	}
}
