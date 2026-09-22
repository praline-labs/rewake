package cache

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
)

// registry is a stand-in for npm: one package version, one tarball, and a
// count of every request, so a test can say the network was not touched.
type registry struct {
	server   *httptest.Server
	requests atomic.Int64
	tarball  []byte
	// integrity is what the version document claims; a test corrupts it to
	// see the download refused.
	integrity string
	// cut, when set, closes the tarball response halfway.
	cut bool
}

func newRegistry(t *testing.T, files map[string]string) *registry {
	t.Helper()
	var entries []entry
	for name, body := range files {
		entries = append(entries, entry{name: name, body: body})
	}
	return newRegistryOf(t, entries)
}

// entry is one member of a test tarball, in order; link, when set, makes it a
// symbolic link to that target.
type entry struct {
	name, body, link string
	// mode is the member's mode; zero is 0755.
	mode int64
}

func newRegistryOf(t *testing.T, entries []entry) *registry {
	t.Helper()
	r := &registry{tarball: tarball(t, entries)}
	sum := sha512.Sum512(r.tarball)
	r.integrity = "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		r.requests.Add(1)
		switch {
		case strings.HasSuffix(request.URL.Path, ".tgz"):
			if r.cut {
				w.Header().Set("Content-Length", "100000")
				_, _ = w.Write(r.tarball[:len(r.tarball)/2])
				return
			}
			_, _ = w.Write(r.tarball)
		default:
			// The real registry answers /latest with the whole version
			// document of the version the tag names.
			version := request.URL.Path[strings.LastIndex(request.URL.Path, "/")+1:]
			if version == Latest {
				version = "9.9.9"
			}
			if strings.HasPrefix(version, "4.0.4") {
				http.NotFound(w, request)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": version,
				"dist":    map[string]any{"tarball": r.server.URL + "/x.tgz", "integrity": r.integrity, "unpackedSize": 10},
			})
		}
	}))
	t.Cleanup(r.server.Close)
	return r
}

func tarball(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(compressed)
	for _, one := range entries {
		mode := one.mode
		if mode == 0 {
			mode = 0o755
		}
		header := &tar.Header{Name: one.name, Mode: mode, Size: int64(len(one.body)), Typeflag: tar.TypeReg}
		if one.link != "" {
			header = &tar.Header{Name: one.name, Mode: 0o777, Linkname: one.link, Typeflag: tar.TypeSymlink}
		}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(one.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func claudeLike() Harness { h, _ := Lookup("claude"); return h }

// storeFor puts the cache one level below a directory of its own, so a test
// can look above the cache for anything an archive wrote there.
func storeFor(t *testing.T, r *registry) *Store {
	return &Store{Root: filepath.Join(t.TempDir(), "cache"), Registry: r.server.URL}
}

// The owner's request, in a test: a version is downloaded once, and the second
// call answers from the cache without a single request.
func TestASecondGetTouchesNoNetwork(t *testing.T) {
	r := newRegistry(t, map[string]string{"package/claude": "binary", "package/package.json": "{}"})
	store := storeFor(t, r)
	first, err := store.Get(context.Background(), claudeLike(), "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Downloaded {
		t.Fatal("the first call did not download")
	}
	if body, err := os.ReadFile(first.Executable); err != nil || string(body) != "binary" {
		t.Fatalf("executable %s: %q, %v", first.Executable, body, err)
	}
	before := r.requests.Load()
	second, err := store.Get(context.Background(), claudeLike(), "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if second.Downloaded || r.requests.Load() != before {
		t.Fatalf("the second call used the network: downloaded=%v, requests %d then %d", second.Downloaded, before, r.requests.Load())
	}
	if second.Executable != first.Executable {
		t.Fatalf("the second call answered %s, the first %s", second.Executable, first.Executable)
	}
}

// A download that does not match its integrity leaves nothing behind that
// could be taken for a version.
func TestAMismatchedDigestCachesNothing(t *testing.T) {
	r := newRegistry(t, map[string]string{"package/claude": "binary"})
	other := sha512.Sum512([]byte("something else"))
	r.integrity = "sha512-" + base64.StdEncoding.EncodeToString(other[:])
	store := storeFor(t, r)
	if _, err := store.Get(context.Background(), claudeLike(), "1.2.3"); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("a mismatched digest was accepted: %v", err)
	}
	assertEmpty(t, store)
}

// A transfer cut halfway is never a cached version, which is the point of
// downloading into a temporary directory and renaming.
func TestAnInterruptedDownloadIsNotAVersion(t *testing.T) {
	r := newRegistry(t, map[string]string{"package/claude": strings.Repeat("x", 50000)})
	r.cut = true
	store := storeFor(t, r)
	if _, err := store.Get(context.Background(), claudeLike(), "1.2.3"); err == nil {
		t.Fatal("a cut transfer was accepted")
	}
	assertEmpty(t, store)
}

// A directory a killed fetch left behind is listed as unfinished, never
// answered as the version, and removed with it.
func TestAnUnfinishedDirectoryIsListedAndNeverUsed(t *testing.T) {
	r := newRegistry(t, map[string]string{"package/claude": "binary"})
	store := storeFor(t, r)
	leftover := filepath.Join(store.Root, "claude", partialPrefix+"1.2.3-abc")
	if err := os.MkdirAll(filepath.Join(leftover, "package"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Cached(claudeLike(), "1.2.3"); err == nil {
		t.Fatal("an unfinished directory was taken for the version")
	}
	entries, err := store.List()
	if err != nil || len(entries) != 1 || !entries[0].Partial || entries[0].Version != "1.2.3" {
		t.Fatalf("listing: %+v, %v", entries, err)
	}
	removed, err := store.Remove(claudeLike(), "1.2.3")
	if err != nil || len(removed) != 1 {
		t.Fatalf("removal: %v, %v", removed, err)
	}
	if _, err := store.Remove(claudeLike(), "1.2.3"); err == nil {
		t.Fatal("removing what is not there reported success")
	}
}

// A package whose layout moved is a refusal naming the missing path, not a
// cached version without its executable.
func TestAMovedExecutableIsARefusal(t *testing.T) {
	r := newRegistry(t, map[string]string{"package/elsewhere": "binary"})
	store := storeFor(t, r)
	if _, err := store.Get(context.Background(), claudeLike(), "1.2.3"); err == nil || !strings.Contains(err.Error(), "package/claude") {
		t.Fatalf("a package without the executable was accepted: %v", err)
	}
	assertEmpty(t, store)
}

// An archive naming a path outside the package is refused before anything is
// written there.
func TestAnEscapingNameIsRefused(t *testing.T) {
	r := newRegistry(t, map[string]string{"../outside": "x", "package/claude": "binary"})
	store := storeFor(t, r)
	if _, err := store.Get(context.Background(), claudeLike(), "1.2.3"); err == nil {
		t.Fatal("an escaping name was accepted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(store.Root), "outside")); err == nil {
		t.Fatal("the escaping file was written")
	}
}

func TestLatestAsksTheRegistry(t *testing.T) {
	r := newRegistry(t, map[string]string{"package/claude": "binary"})
	store := storeFor(t, r)
	version, err := store.Resolve(context.Background(), claudeLike(), Latest)
	if err != nil || version != "9.9.9" {
		t.Fatalf("latest resolved to %q, %v", version, err)
	}
}

// "installed" asks the binary on PATH and nothing else.
func TestInstalledAsksTheBinaryOnPath(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\necho 'codex-cli 0.155.1'\n"
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	codex, _ := Lookup("codex")
	version, err := (&Store{}).Resolve(context.Background(), codex, Installed)
	if err != nil || version != "0.155.1" {
		t.Fatalf("installed resolved to %q, %v", version, err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (&Store{}).Resolve(context.Background(), codex, Installed); err == nil {
		t.Fatal("no binary on PATH resolved to a version")
	}
}

func TestASelectorCannotNameAPath(t *testing.T) {
	codex, _ := Lookup("codex")
	for _, selector := range []string{"../x", "1.2", "1.2.3/..", "next", ""} {
		if _, err := (&Store{}).Resolve(context.Background(), codex, selector); err == nil {
			t.Errorf("selector %q was accepted", selector)
		}
	}
}

// assertEmpty requires that a failed fetch left nothing: no version or
// unfinished directory in the listing, no file anywhere under the cache, and
// nothing beside the cache in the directory above it.
func assertEmpty(t *testing.T, store *Store) {
	t.Helper()
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("the cache holds %+v after a failed fetch", entries)
	}
	above := filepath.Dir(store.Root)
	_ = filepath.WalkDir(above, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == above || path == store.Root || path == filepath.Join(store.Root, "claude") {
			return nil
		}
		t.Errorf("a failed fetch left %s", path)
		return nil
	})
}

// The review's hostile archive: each link lexically points inside the package
// from its own directory, but the second is created through the first, and
// the file lands one level above wherever unpacking happens. With a correct
// digest it reaches the unpacker, which must refuse links outright.
func hostileLinks() []entry {
	return []entry{
		{name: "package/t", link: ".."},
		{name: "package/t/u", link: ".."},
		{name: "package/t/u/ESCAPED", body: "out"},
		{name: "package/claude", body: "binary"},
	}
}

func TestAChainOfLinksIsRefusedAndWritesNothing(t *testing.T) {
	r := newRegistryOf(t, hostileLinks())
	store := storeFor(t, r)
	_, err := store.Get(context.Background(), claudeLike(), "1.2.3")
	if err == nil || !strings.Contains(err.Error(), "links") {
		t.Fatalf("an archive of chained links was accepted: %v", err)
	}
	assertEmpty(t, store)
}

// The same archive under a digest that does not match is refused before a
// single entry is unpacked.
func TestAHostileArchiveWithAWrongDigestIsNeverUnpacked(t *testing.T) {
	r := newRegistryOf(t, hostileLinks())
	other := sha512.Sum512([]byte("not this archive"))
	r.integrity = "sha512-" + base64.StdEncoding.EncodeToString(other[:])
	store := storeFor(t, r)
	_, err := store.Get(context.Background(), claudeLike(), "1.2.3")
	if err == nil || !strings.Contains(err.Error(), "nothing was unpacked") {
		t.Fatalf("a mismatched digest was not refused before unpacking: %v", err)
	}
	assertEmpty(t, store)
}

// The refusal names the version as the caller typed it, not the platform
// package it maps to.
func TestAnUnknownVersionIsNamedAsTyped(t *testing.T) {
	r := newRegistry(t, map[string]string{"package/claude": "binary"})
	store := storeFor(t, r)
	_, err := store.Get(context.Background(), claudeLike(), "4.0.4")
	if err == nil || !strings.Contains(err.Error(), "no claude 4.0.4;") || strings.Contains(err.Error(), "musl") {
		t.Fatalf("the refusal does not name the typed version: %v", err)
	}
	assertEmpty(t, store)
}

// A version is one user's: its directory is 0700, and no file in it is
// writable by anyone else, even when the archive says 0777 and the umask
// takes nothing away.
func TestAVersionIsPrivateWhateverTheArchiveAndUmaskSay(t *testing.T) {
	previous := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(previous) })
	r := newRegistryOf(t, []entry{{name: "package/claude", body: "binary", mode: 0o777}})
	store := storeFor(t, r)
	version, err := store.Get(context.Background(), claudeLike(), "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.Stat(version.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dir.Mode().Perm(); got != 0o700 {
		t.Errorf("the version directory is %o, want 700", got)
	}
	file, err := os.Stat(version.Executable)
	if err != nil {
		t.Fatal(err)
	}
	if got := file.Mode().Perm(); got != 0o755 {
		t.Errorf("an archive member of 0777 was written %o, want 755", got)
	}
}
