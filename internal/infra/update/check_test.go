package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/parts"
)

// registry is npm as a test answers it, with no network: latest names a
// release, and pack puts that release's entry where it was asked.
type registry struct {
	latest   string
	releases []string
	offline  bool
	calls    [][]string
}

func (r *registry) run(_ context.Context, _ string, args []string, stdout, stderr io.Writer) error {
	r.calls = append(r.calls, args)
	if r.offline {
		_, _ = fmt.Fprintln(stderr, "npm error code ECONNREFUSED")
		return errors.New("exit status 1")
	}
	spec := args[1]
	version := strings.TrimPrefix(spec, Package+"@")
	if version == "latest" {
		version = r.latest
	}
	if !slices.Contains(r.releases, version) {
		_, _ = fmt.Fprint(stdout, `{"error":{"code":"E404","summary":"No match found for version `+version+`"}}`)
		return errors.New("exit status 1")
	}
	switch args[0] {
	case "view":
		_, _ = fmt.Fprintf(stdout, "%q\n", version)
	case "pack":
		name := "praline-labs-rewake-" + version + ".tgz"
		if err := os.WriteFile(filepath.Join(args[3], name), entry(version), 0o600); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, `[{"filename":%q}]`, name)
	}
	return nil
}

// entry is a release's entry tarball: its changelog and this platform's
// manifest, as tools/release packs them.
func entry(version string) []byte {
	var buf bytes.Buffer
	zipped := gzip.NewWriter(&buf)
	archive := tar.NewWriter(zipped)
	m := parts.Manifest{Parts: []parts.Part{{Name: parts.ShellPart, Class: parts.Shell, Digest: "sh-" + version}}}
	for name, data := range map[string][]byte{
		"package/" + ChangelogFile:                             []byte("## " + version + "\n### shell\n- New.\n"),
		"package/" + ManifestsDir + "/" + Platform() + ".json": parts.Encode(m),
		"package/README.md":                                    []byte("readme"),
	} {
		_ = archive.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg})
		_, _ = archive.Write(data)
	}
	_ = archive.Close()
	_ = zipped.Close()
	return buf.Bytes()
}

// stateRoot is a state root as a launch makes it.
func stateRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "v2")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

// A check fetches a new release's manifest and changelog once, keeps them
// through a failed check, and asks npm the install's way every time.
func TestCheckFetchesOnceAndKeepsTheLastGoodAnswer(t *testing.T) {
	root := stateRoot(t)
	reg := &registry{latest: "2.13.0", releases: []string{"2.12.0", "2.13.0"}}
	npm := NPM{Prefix: "/opt/node", Run: reg.run}
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	record, err := Check(context.Background(), root, npm, now)
	if err != nil {
		t.Fatal(err)
	}
	if record.Latest != "2.13.0" || record.Manifest == nil || len(record.Changelog) != 1 {
		t.Fatalf("record %+v", record)
	}
	if shell, _ := record.Manifest.Find(parts.ShellPart); shell.Digest != "sh-2.13.0" {
		t.Errorf("the manifest's shell is %+v", shell)
	}
	if read := ReadRecord(root); read.Latest != "2.13.0" || !read.Checked.Equal(now) {
		t.Errorf("the record on disk is %+v", read)
	}
	for _, args := range reg.calls {
		if !slices.Equal(args[len(args)-3:], []string{"--global", "--prefix", "/opt/node"}) {
			t.Errorf("npm %v is not asked as npm install -g is", args)
		}
	}
	reg.calls = nil
	if _, err := Check(context.Background(), root, npm, now.Add(Every)); err != nil || len(reg.calls) != 1 || reg.calls[0][0] != "view" {
		t.Errorf("the same latest fetched again: %v, %v", reg.calls, err)
	}
	reg.offline = true
	later := now.Add(2 * Every)
	record, err = Check(context.Background(), root, npm, later)
	if err == nil || record.Latest != "2.13.0" || record.Manifest == nil || record.Error == "" || !record.Checked.Equal(later) {
		t.Errorf("offline: %+v, %v", record, err)
	}
	if ReadRecord(root).Due(later.Add(time.Minute)) {
		t.Error("a failed check is asked again before its time")
	}
}

// update <version> installs only a release the registry has: E404 is no
// such release, not a failure to ask.
func TestExistsTellsNoSuchReleaseFromAFailure(t *testing.T) {
	reg := &registry{latest: "2.13.0", releases: []string{"2.13.0"}}
	npm := NPM{Prefix: "/opt/node", Run: reg.run}
	if err := npm.Exists(context.Background(), "2.13.0"); err != nil {
		t.Errorf("2.13.0: %v", err)
	}
	if err := npm.Exists(context.Background(), "9.9.9"); !errors.Is(err, ErrNoRelease) {
		t.Errorf("9.9.9: %v, want no such release", err)
	}
	reg.offline = true
	if err := npm.Exists(context.Background(), "2.13.0"); err == nil || errors.Is(err, ErrNoRelease) {
		t.Errorf("offline: %v, want a failure to ask", err)
	}
}
