package build

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The build id is the content of the image the process runs — no version
// line, no path, no inode: a byte-identical copy is the same build and one
// byte more is another.
func TestBuildIDIsExecutableContent(t *testing.T) {
	data, err := os.ReadFile("/proc/self/exe")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])
	got, err := ID()
	if err != nil || got != want {
		t.Fatalf("ID() = %q, %v; the image's SHA-256 is %q", got, err, want)
	}
	copied := filepath.Join(t.TempDir(), "copy")
	if err := os.WriteFile(copied, data, 0o700); err != nil {
		t.Fatal(err)
	}
	if theirs, err := FileID(copied); err != nil || theirs != want {
		t.Fatalf("a byte-identical copy: %q, %v", theirs, err)
	}
	changed := filepath.Join(t.TempDir(), "changed")
	if err := os.WriteFile(changed, append(data, 0), 0o700); err != nil {
		t.Fatal(err)
	}
	if theirs, err := FileID(changed); err != nil || theirs == want {
		t.Fatalf("an image one byte longer: %q, %v", theirs, err)
	}
	// The running image, not the file at its path: a process whose file was
	// replaced and then removed before it hashed still hashes what it runs.
	cmd := exec.Command(copied, "-test.run=^$")
	cmd.Env = append(os.Environ(), holderEnv+"=id")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(changed, copied); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(copied); err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(stdin, "go\n")
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil || strings.TrimSpace(out.String()) != want {
		t.Fatalf("a process whose file was replaced and removed: %q, %v", out.String(), err)
	}
	if Short(got) != want[:12] {
		t.Fatalf("Short = %q", Short(got))
	}
}

// An image that cannot be read gives no id, and neither a writer nor a
// launch creates anything before refusing: the base stays unmade.
func TestUnreadableImageHasNoEffects(t *testing.T) {
	setImage(t, filepath.Join(t.TempDir(), "gone"))
	base := newBase(t)
	for name, acquire := range map[string]func(string, string) (*Lease, error){"writer": Acquire, "launch": AcquireLaunch} {
		lease, err := acquire(base, "default")
		if err == nil {
			lease.Release()
			t.Fatalf("%s: acquired without an id", name)
		}
		if !strings.Contains(err.Error(), "cannot read its own executable") {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := os.Lstat(base); !os.IsNotExist(err) {
			t.Fatalf("%s made the base before it had an id: %v", name, err)
		}
	}
}
