// Package scripts holds the tests of the shell scripts that package rewake.
package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// platformPackage is the npm package the shim looks for on this machine.
func platformPackage(t *testing.T) string {
	t.Helper()
	architecture := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	if runtime.GOOS != "linux" || architecture == "" {
		t.Skipf("no rewake package for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return "rewake-linux-" + architecture
}

// install lays out the entry package with the shim under a node_modules path.
func install(t *testing.T, root, at string) string {
	t.Helper()
	shim, err := os.ReadFile("shim.sh")
	if err != nil {
		t.Fatalf("read shim: %v", err)
	}
	bin := filepath.Join(root, at, "@iiiokojiadbi", "rewake", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(bin, "rewake")
	if err := os.WriteFile(path, shim, 0o755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	return path
}

// platform lays out a platform package whose binary says who it is.
func platform(t *testing.T, root, at, name, says string) {
	t.Helper()
	bin := filepath.Join(root, at, "@iiiokojiadbi", name, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	script := "#!/bin/sh\necho " + says + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "rewake"), []byte(script), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
}

func runShim(t *testing.T, path string, args ...string) (string, string, error) {
	t.Helper()
	command := exec.Command(path, args...)
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

// The package's own dependency wins over a copy beside it, which may be left
// from an earlier install.
func TestTheOwnDependencyWins(t *testing.T) {
	name := platformPackage(t)
	root := t.TempDir()
	shim := install(t, root, "node_modules")
	platform(t, root, "node_modules/@iiiokojiadbi/rewake/node_modules", name, "own")
	platform(t, root, "node_modules", name, "stale")

	out, errOut, err := runShim(t, shim, "--version")
	if err != nil || strings.TrimSpace(out) != "own --version" {
		t.Errorf("out = %q, err = %v %q; want the nested binary with the arguments", out, err, errOut)
	}
}

// npm may install the entry deep in the tree and hoist its platform package
// several levels up. The shim has to find it there, as Node would.
func TestAHoistedDependencyIsFound(t *testing.T) {
	name := platformPackage(t)
	root := t.TempDir()
	shim := install(t, root, "node_modules/consumer/node_modules")
	platform(t, root, "node_modules", name, "hoisted")

	out, errOut, err := runShim(t, shim)
	if err != nil || strings.TrimSpace(out) != "hoisted" {
		t.Errorf("out = %q, err = %v %q; want the hoisted binary", out, err, errOut)
	}
}

// npm puts the command in node_modules/.bin as a symlink to the shim.
func TestTheShimRunsThroughTheBinLink(t *testing.T) {
	name := platformPackage(t)
	root := t.TempDir()
	install(t, root, "node_modules")
	platform(t, root, "node_modules", name, "linked")
	link := filepath.Join(root, "node_modules", ".bin", "rewake")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink("../@iiiokojiadbi/rewake/bin/rewake", link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	out, errOut, err := runShim(t, link)
	if err != nil || strings.TrimSpace(out) != "linked" {
		t.Errorf("out = %q, err = %v %q; want the binary found through the link", out, err, errOut)
	}
}

func TestAMissingBinaryIsNamed(t *testing.T) {
	name := platformPackage(t)
	shim := install(t, t.TempDir(), "node_modules")

	_, errOut, err := runShim(t, shim)
	if err == nil || !strings.Contains(errOut, "npm install -g @iiiokojiadbi/"+name) {
		t.Errorf("err = %v, stderr = %q; want a failure that names the package to install", err, errOut)
	}
}
