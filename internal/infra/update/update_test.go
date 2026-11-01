package update

import (
	"os"
	"path/filepath"
	"testing"
)

// The release is offered only when it is newer by its numbers, never by
// the text: 2.10.0 is newer than 2.9.1.
func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"2.12.0", "2.11.0", true},
		{"2.10.0", "2.9.1", true},
		{"3.0.0", "2.99.99", true},
		{"2.11.0", "2.11.0", false},
		{"2.11.0", "2.12.0", false},
		{"2.12.0-linux-x64", "2.11.0", false},
		{"2.12.0-rc.1", "2.11.0", false},
		{"latest", "2.11.0", false},
		{"2.12.0", "", true},
		{"02.12.0", "2.11.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// Only a build that pack.sh marked as npm's checks the registry; any other
// build — tools/land's, one by hand — updates by being built again.
func TestOnlyABuildFromNPMUpdates(t *testing.T) {
	was := Channel
	t.Cleanup(func() { Channel = was })
	for channel, want := range map[string]bool{"": false, "npm": true, "land": false} {
		Channel = channel
		if FromNPM() != want {
			t.Errorf("Channel %q: FromNPM %v, want %v", channel, !want, want)
		}
	}
}

func TestPlatformBuild(t *testing.T) {
	for version, want := range map[string]string{"2.12.0-linux-x64": "2.12.0", "2.12.0-linux-arm64": "2.12.0", "2.12.0": "", "2.12.0-rc.1": ""} {
		release, ok := PlatformBuild(version)
		if ok != (want != "") || ok && release != want {
			t.Errorf("PlatformBuild(%q) = %q, %v; want %q", version, release, ok, want)
		}
	}
}

// npm installs under the prefix the running rewake lies in, nested or
// hoisted, and not into another node's.
func TestPrefixOfAnInstall(t *testing.T) {
	for origin, want := range map[string]string{
		"/usr/lib/node_modules/@praline-labs/rewake/node_modules/@praline-labs/rewake-linux-x64/bin": "/usr",
		"/home/u/.nvm/versions/node/v22.19.0/lib/node_modules/@praline-labs/rewake-linux-x64/bin":    "/home/u/.nvm/versions/node/v22.19.0",
	} {
		if got, err := Prefix(origin); err != nil || got != want {
			t.Errorf("Prefix(%q) = %q, %v; want %q", origin, got, err, want)
		}
	}
	if _, err := Prefix("/home/u/.local/share/rewake"); err == nil {
		t.Error("a build from source has a prefix")
	}
}

// The installed release is the platform build's version without its
// platform; anything else is no npm install of rewake.
func TestInstalledReadsThePlatformBuild(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(content string) {
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"name":"@praline-labs/rewake","version":"2.12.0-linux-x64"}`)
	if got, err := Installed(bin); err != nil || got != "2.12.0" {
		t.Errorf("Installed = %q, %v", got, err)
	}
	write(`{"name":"other","version":"2.12.0-linux-x64"}`)
	if _, err := Installed(bin); err == nil {
		t.Error("another package's bin reads as rewake's")
	}
}
