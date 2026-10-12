package proc

import (
	"os"
	"path/filepath"
	"testing"
)

// The environment a process started with is read whole, by its names; an
// environment that does not read is an error, never an empty one.
func TestInitialEnvReadsTheEnvironmentAsStarted(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "7")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "environ"), []byte("A=1\x00REWAKE_SESSIONS=x\x00REWAKE_SESSION=\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := Reader{Root: root}
	for key, want := range map[string]bool{"A": true, "REWAKE_SESSION": true, "B": false, "REWAKE": false} {
		if got, err := r.InitialEnv(7, key); err != nil || got != want {
			t.Fatalf("%s: %v, %v", key, got, err)
		}
	}
	if _, err := r.InitialEnv(8, "A"); err == nil {
		t.Fatal("a missing environment read")
	}
	if err := os.WriteFile(filepath.Join(dir, "environ"), make([]byte, maxEnvironBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.InitialEnv(7, "A"); err == nil {
		t.Fatal("an environment past the bound read")
	}
	if found, err := Default.InitialEnv(os.Getpid(), "PATH"); err != nil || found != (os.Getenv("PATH") != "") {
		t.Fatalf("this process's PATH: %v, %v", found, err)
	}
}

// The user of a process is its real and effective one, from its status.
func TestOwnerReadsTheUsersOfAProcess(t *testing.T) {
	realUID, effectiveUID, err := Default.Owner(os.Getpid())
	if err != nil || realUID != os.Getuid() || effectiveUID != os.Geteuid() {
		t.Fatalf("this process is %d/%d, %v", realUID, effectiveUID, err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "7"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "7", "status"), []byte("Name:\tx\nUid:\tnone\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (Reader{Root: root}).Owner(7); err == nil {
		t.Fatal("a status naming no user read")
	}
}
