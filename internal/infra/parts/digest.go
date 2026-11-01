package parts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// FileDigest is the SHA-256 of a file's content, as the build id is of an
// executable's.
func FileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// SourcesDigest is a digest over the files of a TypeScript part's directory
// that it runs from: every file but node_modules, its tests and hidden ones.
// Paths count with the content, so a rename is a change.
func SourcesDigest(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if path != dir && (name == "node_modules" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || strings.Contains(name, ".test.") || !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	return digestOf(dir, files, nil, nil)
}

// ShellDigest is the digest of the shell a launch would start at path: a
// checkout's rewake-shell, which runs its sources with bun, is the digest of
// those sources; a compiled shell is its file's.
func ShellDigest(path string) (digest, file string, err error) {
	dir := filepath.Dir(path)
	if filepath.Base(path) == "rewake-shell" {
		if _, err := os.Stat(filepath.Join(dir, "src", "main.tsx")); err == nil {
			digest, err := SourcesDigest(dir)
			return digest, "", err
		}
	}
	file, err = FileDigest(path)
	return "", file, err
}

// VersionFile holds the version a build from source reports, which a release
// commit must set to the released one (tools/release); VersionLine is its
// declaration there, the release gate's and the digest's one expression.
const VersionFile = "internal/cli/registry.go"

// VersionLine matches the declaration of the version in VersionFile.
var VersionLine = regexp.MustCompile(`(?m)^var Version = "([^"]*)"$`)

// GoDigest is a digest over what go build puts into ./cmd/rewake for a
// platform: the Go and embedded files of every package of the module it
// depends on, go.mod, go.sum and the toolchain's version. The build time, the
// VCS stamp and the release version are left out, so two builds of one tree
// have one digest, and so have two releases whose sources differ only in the
// version a release must raise (docs/install.md#the-parts-and-what-an-update-restarts).
// goos and goarch empty are this machine's.
func GoDigest(ctx context.Context, root, goos, goarch string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	list := exec.CommandContext(ctx, "go", "list", "-deps", "-json=Dir,Module,GoFiles,CgoFiles,EmbedFiles", "./cmd/rewake")
	list.Dir = root
	list.Env = os.Environ()
	if goos != "" {
		list.Env = append(list.Env, "GOOS="+goos, "GOARCH="+goarch)
	}
	var stderr bytes.Buffer
	list.Stderr = &stderr
	out, err := list.Output()
	if err != nil {
		return "", fmt.Errorf("go list: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	version := exec.CommandContext(ctx, "go", "env", "GOVERSION")
	version.Dir = root
	goVersion, err := version.Output()
	if err != nil {
		return "", fmt.Errorf("go env GOVERSION: %w", err)
	}
	files := []string{"go.mod", "go.sum"}
	decoder := json.NewDecoder(bytes.NewReader(out))
	for decoder.More() {
		var pkg struct {
			Dir                           string
			Module                        *struct{ Main bool }
			GoFiles, CgoFiles, EmbedFiles []string
		}
		if err := decoder.Decode(&pkg); err != nil {
			return "", fmt.Errorf("go list's answer: %w", err)
		}
		if pkg.Module == nil || !pkg.Module.Main {
			// The standard library is the toolchain's, other modules go.sum's.
			continue
		}
		rel, err := filepath.Rel(root, pkg.Dir)
		if err != nil {
			return "", err
		}
		for _, name := range slices.Concat(pkg.GoFiles, pkg.CgoFiles, pkg.EmbedFiles) {
			files = append(files, filepath.ToSlash(filepath.Join(rel, name)))
		}
	}
	platform := strings.TrimSpace(string(goVersion)) + " " + goos + "/" + goarch
	return digestOf(root, files, []byte(platform), map[string]func([]byte) ([]byte, error){VersionFile: versionless})
}

// versionless is VersionFile with its one version line made empty; every
// other line counts as it is. No such line, or two, fails: the line is the
// release gate's contract, and a digest that silently kept a version would
// ask every release for a room restart.
func versionless(data []byte) ([]byte, error) {
	if n := len(VersionLine.FindAllIndex(data, -1)); n != 1 {
		return nil, fmt.Errorf("%s has %d lines var Version = \"...\", not one", VersionFile, n)
	}
	return VersionLine.ReplaceAll(data, []byte(`var Version = ""`)), nil
}

// digestOf hashes each file's path and content, in path order, after extra;
// a file named in normalize is hashed as that function rewrites it.
func digestOf(root string, files []string, extra []byte, normalize map[string]func([]byte) ([]byte, error)) (string, error) {
	slices.Sort(files)
	files = slices.Compact(files)
	sum := sha256.New()
	sum.Write(extra)
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		if rewrite := normalize[rel]; rewrite != nil {
			if data, err = rewrite(data); err != nil {
				return "", err
			}
		}
		_, _ = fmt.Fprintf(sum, "\x00%s\x00%d\x00", rel, len(data))
		sum.Write(data)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
