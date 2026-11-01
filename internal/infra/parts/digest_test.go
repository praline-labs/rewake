package parts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// module writes a module whose ./cmd/rewake imports a VersionFile with the
// given declaration and body, and answers its root.
func module(t *testing.T, version, body string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":             "module example.com/r\n\ngo 1.25\n",
		"go.sum":             "",
		"cmd/rewake/main.go": "package main\n\nimport \"example.com/r/internal/cli\"\n\nfunc main() { println(cli.Version) }\n",
		VersionFile:          "package cli\n\n// Version is the release.\n" + version + "\n" + body,
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// A release must raise the version line, and a shell-only release must not
// ask for a room restart: two releases whose sources differ only there have
// one digest, and any other change of that file is still a change (found in
// review of the update's design, November 1, 2026).
func TestGoDigestLeavesTheReleaseVersionOut(t *testing.T) {
	ctx := context.Background()
	digest := func(root string) string {
		t.Helper()
		d, err := GoDigest(ctx, root, "", "")
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	body := "var other = 1\n"
	older := digest(module(t, `var Version = "2.11.0"`, body))
	newer := digest(module(t, `var Version = "2.12.0"`, body))
	if older != newer {
		t.Errorf("2.11.0 and 2.12.0 of one tree digest %s and %s", older, newer)
	}
	if changed := digest(module(t, `var Version = "2.12.0"`, "var other = 2\n")); changed == newer {
		t.Error("another line of the version file changed and the digest did not")
	}
	for _, lines := range []string{"var other = 1", `var Version = "2.12.0"` + "\n" + `var Version = "2.12.1"`} {
		if _, err := GoDigest(ctx, module(t, lines, ""), "", ""); err == nil || !strings.Contains(err.Error(), "not one") {
			t.Errorf("a version file with %q digests: %v", lines, err)
		}
	}
}
