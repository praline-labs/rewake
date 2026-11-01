package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/infra/parts"
)

// The bounds of npm's calls: asking the registry and fetching an entry are
// small, an install downloads a build.
const (
	askWithin     = 30 * time.Second
	installWithin = 10 * time.Minute
	// entryLimit bounds what is read of an entry's tarball: a few KB ship
	// in it, the builds never.
	entryLimit = 16 << 20
)

// ManifestsDir is where a release's entry package carries the manifest of
// each platform build, the same bytes as that build's bin/manifest.json, so
// what a release restarts is known before its build is fetched.
const ManifestsDir = "manifests"

// Runner runs npm with args in dir, its output to stdout and stderr. The
// tests replace it; nil is the npm on PATH, with this process's environment,
// which carries the proxy every launch has.
type Runner func(ctx context.Context, dir string, args []string, stdout, stderr io.Writer) error

// NPM is npm as an install of rewake is updated with: every call takes
// --global and the install's own --prefix, so the check, the fetch and the
// install read one configuration — the person's .npmrc, the prefix's global
// npmrc and the environment, never a project's .npmrc, which npm install -g
// ignores — and the preview cannot come from another registry than the
// install.
type NPM struct {
	Prefix string
	Run    Runner
}

// Error is a call of npm that failed, with npm's own code when it said one.
type Error struct {
	Code, Why string
}

func (e *Error) Error() string {
	if e.Code != "" && !strings.Contains(e.Why, e.Code) {
		return "npm " + e.Code + ": " + e.Why
	}
	return "npm: " + e.Why
}

// ErrNoRelease is a version the registry does not have.
var ErrNoRelease = errors.New("no such release")

func (n NPM) call(ctx context.Context, limit time.Duration, dir string, stdout, stderr io.Writer, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	args = append(args, "--global", "--prefix", n.Prefix)
	run := n.Run
	if run == nil {
		run = system
	}
	err := run(ctx, dir, args, stdout, stderr)
	if err != nil && ctx.Err() != nil {
		return &Error{Why: "no answer within " + limit.String()}
	}
	return err
}

func system(ctx context.Context, dir string, args []string, stdout, stderr io.Writer) error {
	command := exec.CommandContext(ctx, "npm", args...)
	command.Dir = dir
	command.Stdout, command.Stderr = stdout, stderr
	return command.Run()
}

// npmCode finds npm's own error code in its log lines, anchored to the line
// npm writes it on.
var npmCode = regexp.MustCompile(`(?m)^npm (?:error|ERR!) code (\S+)\s*$`)

// failed is a failed call as an Error: npm's {"error":{...}} under --json,
// else the code of its log lines, else what it said last.
func failed(err error, stdout, stderr string) *Error {
	var answer struct {
		Error struct{ Code, Summary string } `json:"error"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(stdout)), &answer) == nil && answer.Error.Code != "" {
		return &Error{Code: answer.Error.Code, Why: firstLine(answer.Error.Summary, err)}
	}
	e := &Error{Why: firstLine(stderr, err)}
	if match := npmCode.FindStringSubmatch(stderr); match != nil {
		e.Code = match[1]
	}
	return e
}

func firstLine(text string, err error) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.Contains(line, " code ") {
			return line
		}
	}
	if err != nil {
		return err.Error()
	}
	return "it failed and said nothing"
}

// view asks the registry for spec's version; "" with no error when npm
// matched none.
func (n NPM) view(ctx context.Context, dir, spec string) (string, error) {
	var stdout, stderr bytes.Buffer
	if err := n.call(ctx, askWithin, dir, &stdout, &stderr, "view", spec, "version", "--json"); err != nil {
		var npm *Error
		if errors.As(err, &npm) {
			return "", npm
		}
		return "", failed(err, stdout.String(), stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", nil
	}
	var version string
	if err := json.Unmarshal([]byte(out), &version); err != nil {
		return "", &Error{Why: fmt.Sprintf("npm view %s answered %q, which names no version", spec, out)}
	}
	return version, nil
}

// Latest is the release npm's latest tag names.
func (n NPM) Latest(ctx context.Context) (string, error) {
	version, err := n.view(ctx, os.TempDir(), Package+"@latest")
	if err == nil && version == "" {
		err = &Error{Why: "the registry names no latest release of " + Package}
	}
	return version, err
}

// Exists says the registry has the release version, exactly: npm's E404,
// or no match, is ErrNoRelease.
func (n NPM) Exists(ctx context.Context, version string) error {
	got, err := n.view(ctx, os.TempDir(), Package+"@"+version)
	var npm *Error
	switch {
	case errors.As(err, &npm) && npm.Code == "E404", err == nil && got == "":
		return fmt.Errorf("%w %s of %s on the registry", ErrNoRelease, version, Package)
	case err != nil:
		return err
	case got != version:
		return &Error{Why: fmt.Sprintf("npm answered %s for %s", got, version)}
	}
	return nil
}

// Fetch is what the release version ships of itself in its entry package:
// the manifest of this machine's build and the changelog. The entry is
// fetched with npm pack, which checks its integrity, into a scratch
// directory of the system's, removed after.
func (n NPM) Fetch(ctx context.Context, version string) (parts.Manifest, []Release, error) {
	dir, err := os.MkdirTemp("", "rewake-update-")
	if err != nil {
		return parts.Manifest{}, nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	var stdout, stderr bytes.Buffer
	spec := Package + "@" + version
	if err := n.call(ctx, askWithin, dir, &stdout, &stderr, "pack", spec, "--pack-destination", dir, "--json"); err != nil {
		var npm *Error
		if errors.As(err, &npm) {
			return parts.Manifest{}, nil, npm
		}
		return parts.Manifest{}, nil, failed(err, stdout.String(), stderr.String())
	}
	var packed []struct{ Filename string }
	if err := json.Unmarshal(stdout.Bytes(), &packed); err != nil || len(packed) != 1 || packed[0].Filename == "" {
		return parts.Manifest{}, nil, &Error{Why: fmt.Sprintf("npm pack %s answered %q, which names no tarball", spec, strings.TrimSpace(stdout.String()))}
	}
	return readEntry(filepath.Join(dir, filepath.Base(packed[0].Filename)), spec)
}

// readEntry reads the manifest and the changelog out of an entry's tarball.
func readEntry(tarball, spec string) (parts.Manifest, []Release, error) {
	f, err := os.Open(tarball)
	if err != nil {
		return parts.Manifest{}, nil, err
	}
	defer func() { _ = f.Close() }()
	files, err := untar(f, "package/"+ChangelogFile, "package/"+path.Join(ManifestsDir, Platform()+".json"))
	if err != nil {
		return parts.Manifest{}, nil, fmt.Errorf("the entry of %s: %w", spec, err)
	}
	changelog, err := ParseChangelog(files[0])
	if err != nil {
		return parts.Manifest{}, nil, fmt.Errorf("the entry of %s: %w", spec, err)
	}
	m, err := parts.Decode(files[1], spec+" "+path.Join(ManifestsDir, Platform()+".json"))
	return m, changelog, err
}

// untar is the content of each name in a gzipped tarball, in names' order;
// a name it does not hold fails.
func untar(r io.Reader, names ...string) ([][]byte, error) {
	zipped, err := gzip.NewReader(io.LimitReader(r, entryLimit))
	if err != nil {
		return nil, err
	}
	out := make([][]byte, len(names))
	found := 0
	archive := tar.NewReader(io.LimitReader(zipped, entryLimit))
	for found < len(names) {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		for i, name := range names {
			if header.Name == name && out[i] == nil && header.Typeflag == tar.TypeReg {
				if out[i], err = io.ReadAll(archive); err != nil {
					return nil, err
				}
				found++
			}
		}
	}
	for i, name := range names {
		if out[i] == nil {
			return nil, fmt.Errorf("it holds no %s", strings.TrimPrefix(name, "package/"))
		}
	}
	return out, nil
}

// Install installs the release version where this rewake was installed,
// npm's output to out.
func (n NPM) Install(ctx context.Context, version string, out io.Writer) error {
	var stderr bytes.Buffer
	err := n.call(ctx, installWithin, os.TempDir(), out, io.MultiWriter(out, &stderr), "install", Package+"@"+version)
	if err != nil {
		var npm *Error
		if errors.As(err, &npm) {
			return npm
		}
		return failed(err, "", stderr.String())
	}
	return nil
}
