// Package container runs a cached harness version in a disposable docker
// container: its own filesystem, a private HOME that vanishes with it, the
// harness mounted read-only, and nothing of the owner's home in sight.
//
// This is the one place in the project where docker is warranted. A harness
// version the owner has not installed is untrusted by definition — the point
// is to learn what it does before it goes near their setup — and a temporary
// directory would still leave it the owner's HOME, their configuration and
// their sessions to read.
package container

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// recipe is the image. Alpine because both harnesses run on musl — Codex is
// built static-pie against it, and the Claude Code musl build needs nothing
// beyond musl itself (readelf on 2.1.280 lists libc.musl alone) — and because
// it is small. Nothing is installed into it, so building needs no network
// beyond the base.
//
// Claude Code's musl build takes its libc from this image, which is one more
// reason the base is pinned: a different musl is a different runtime.
//
// The tag is derived from this text, so an edit here is a new image, built on
// first use; an unchanged recipe reuses the image already built. The base is
// pinned by digest (the alpine:3.22 index verified on September 23, 2026), so
// a tag moved upstream changes nothing until this line is edited.
const recipe = `FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce
LABEL org.opencontainers.image.description="rewake: a disposable place to run a cached harness version"
`

// Image is the tag the recipe builds into.
func Image() string {
	sum := sha256.Sum256([]byte(recipe))
	return "rewake-harness:" + hex.EncodeToString(sum[:])[:12]
}

// Home is the private HOME inside the container, a tmpfs gone when it exits.
const Home = "/home/runner"

// HarnessMount is where the cached version appears, read-only.
const HarnessMount = "/opt/harness"

// Spec is one run.
type Spec struct {
	// Version is the cached version's directory on the host.
	Version string
	// Executable is the binary, relative to Version.
	Executable string
	// Args follow the executable.
	Args []string
	// Writable is a host directory mounted read-write at WritableMount: the
	// only place the container can leave anything behind. Empty for none.
	Writable      string
	WritableMount string
	// Network is docker's network mode; empty is "none". Generating a schema
	// needs no network, and a harness that tries to reach one anyway should
	// be seen failing rather than quietly succeeding.
	Network string
}

// ErrNoDocker is the refusal when docker is not there. It names the next
// action because the caller, an agent, cannot guess it.
var ErrNoDocker = errors.New("docker is not on PATH; running a named harness version needs it — install docker, or run against the installed harness instead")

// buildTimeout bounds building the image, which at most pulls the base.
const buildTimeout = 5 * time.Minute

// Ensure builds the image when it is not present yet, and says which it did.
func Ensure(ctx context.Context) (built bool, err error) {
	docker, err := exec.LookPath("docker")
	if err != nil {
		return false, ErrNoDocker
	}
	inspect := exec.CommandContext(ctx, docker, "image", "inspect", "--format", "{{.Id}}", Image())
	if inspect.Run() == nil {
		return false, nil
	}
	ctx, stop := context.WithTimeout(ctx, buildTimeout)
	defer stop()
	build := exec.CommandContext(ctx, docker, "build", "--tag", Image(), "-")
	build.Stdin = strings.NewReader(recipe)
	if out, err := build.CombinedOutput(); err != nil {
		return false, fmt.Errorf("building the image %s: %v: %s", Image(), err, tail(out))
	}
	return true, nil
}

// Args are the docker arguments for a run, apart from the docker binary. They
// are a function of their own so the isolation can be checked without docker.
func Args(spec Spec, name string) []string {
	network := spec.Network
	if network == "" {
		network = "none"
	}
	args := []string{
		"run", "--rm", "--name", name,
		"--network", network,
		// The image's own filesystem is not writable either: whatever the
		// harness writes goes to the tmpfs HOME, or to the one mount given.
		"--read-only",
		"--tmpfs", "/tmp:mode=1777",
		"--tmpfs", Home + ":mode=1777",
		// The caller's ids, so what lands in the writable mount belongs to
		// the caller rather than to root.
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		// A harness needs no capability and no privilege it could gain, and
		// a runaway one must not take the machine with it.
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--pids-limit=512",
		"--memory=4g",
		"--env", "HOME=" + Home,
		"--workdir", Home,
		"--mount", "type=bind,source=" + spec.Version + ",target=" + HarnessMount + ",readonly",
	}
	if spec.Writable != "" {
		args = append(args, "--mount", "type=bind,source="+spec.Writable+",target="+spec.WritableMount)
	}
	args = append(args, Image(), filepath.Join(HarnessMount, spec.Executable))
	return append(args, spec.Args...)
}

// Run runs one command in a fresh container and answers its combined output.
// A canceled context removes the container as well: killing the docker
// client leaves the container it started running.
func Run(ctx context.Context, spec Spec) ([]byte, error) {
	for _, path := range []string{spec.Version, spec.Writable} {
		if err := Mountable(path); err != nil {
			return nil, err
		}
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		return nil, ErrNoDocker
	}
	if _, err := Ensure(ctx); err != nil {
		return nil, err
	}
	name := "rewake-harness-" + random()
	run := exec.CommandContext(ctx, docker, Args(spec, name)...)
	var out bytes.Buffer
	run.Stdout, run.Stderr = &out, &out
	err = run.Run()
	if ctx.Err() != nil {
		// Bounded as well: a daemon that does not answer must not hold the
		// caller past the deadline it has just missed.
		removeCtx, stop := context.WithTimeout(context.Background(), removeTimeout)
		defer stop()
		_ = exec.CommandContext(removeCtx, docker, "rm", "--force", name).Run()
		return out.Bytes(), fmt.Errorf("the container %s did not finish in time: %w", name, ctx.Err())
	}
	if err != nil {
		return out.Bytes(), fmt.Errorf("the harness in the container failed: %w: %s", err, tail(out.Bytes()))
	}
	return out.Bytes(), nil
}

// removeTimeout bounds removing a container whose run ran out of time.
const removeTimeout = 30 * time.Second

// Mountable refuses a path docker's --mount syntax cannot carry: a comma
// separates its fields, so a path with one would be read as a different
// mount rather than refused.
func Mountable(path string) error {
	if strings.Contains(path, ",") {
		return fmt.Errorf("%q contains a comma, which docker's --mount cannot carry; use a path without one", path)
	}
	return nil
}

func random() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// tail keeps a refusal readable: docker build prints hundreds of lines, and
// the cause is at the end.
func tail(out []byte) string {
	text := strings.TrimSpace(string(out))
	if len(text) > 2000 {
		text = "…" + text[len(text)-2000:]
	}
	return text
}
