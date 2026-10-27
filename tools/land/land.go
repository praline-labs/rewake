package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	gitLimit   = 2 * time.Minute
	buildLimit = 10 * time.Minute
	// branch is the line every accepted step lands on.
	branch = "v2"
)

// lander is one run of land.
type lander struct {
	call
	run runner
	out io.Writer
	// processes lists the command lines running on the machine, for the
	// race run's -p.
	processes func() [][]string
	root      string
	log       io.Writer
	logPath   string
	// old is v2 before the merge, what a red check resets it to.
	old string
}

func (l *lander) say(format string, args ...any) {
	_, _ = fmt.Fprintf(l.out, format+"\n", args...)
}

// git runs git in the checkout.
func (l *lander) git(ctx context.Context, args ...string) result {
	return l.run(ctx, command{dir: l.root, name: "git", args: args, limit: gitLimit}, l.log)
}

// refuse ends a run that changed nothing.
func (l *lander) refuse(format string, args ...any) int {
	l.say("refused: "+format, args...)
	if l.logPath != "" {
		l.say("log: %s", l.logPath)
	}
	return exitFailed
}

// land runs the steps and returns the exit code.
func (l *lander) land(ctx context.Context) int {
	l.log = io.Discard
	if l.root == "" {
		r := l.git(ctx, "rev-parse", "--show-toplevel")
		if !r.ok() {
			return l.refuse("not inside a git checkout: %s", r.why())
		}
		l.root = strings.TrimSpace(r.stdout)
	}
	closeLog, err := l.openLog()
	if err != nil {
		return l.refuse("%v", err)
	}
	defer closeLog()

	if r := l.git(ctx, "symbolic-ref", "--quiet", "--short", "HEAD"); !r.ok() || strings.TrimSpace(r.stdout) != branch {
		return l.refuse("%s has %s checked out, not %s: land runs from the main checkout on %s", l.root, describe(r), branch, branch)
	}
	if r := l.git(ctx, "status", "--porcelain"); !r.ok() {
		return l.refuse("could not read the working tree: %s", r.why())
	} else if strings.TrimSpace(r.stdout) != "" {
		return l.refuse("the working tree has changes, which a reset after a red check would lose: %s", firstLines(r.stdout, 3))
	}
	if !l.noPush {
		if info, err := os.Stat(filepath.Join(l.install, "rewake")); err != nil || !info.Mode().IsRegular() {
			return l.refuse("no build to replace at %s: name the install with --install <dir>, or land with --no-push", filepath.Join(l.install, "rewake"))
		}
	}
	trailers, err := coAuthors(l.root)
	if err != nil {
		return l.refuse("%v", err)
	}
	r := l.git(ctx, "rev-parse", "--verify", "HEAD")
	if !r.ok() {
		return l.refuse("could not read %s: %s", branch, r.why())
	}
	l.old = strings.TrimSpace(r.stdout)
	r = l.git(ctx, "rev-parse", "--verify", "--quiet", l.target+"^{commit}")
	if !r.ok() {
		l.say("land: %s names no commit here", l.target)
		return exitCall
	}
	target := strings.TrimSpace(r.stdout)

	if l.git(ctx, "merge-base", "--is-ancestor", target, l.old).ok() {
		l.say("nothing to land: %s %s is already in %s %s", l.target, short(target), branch, short(l.old))
		return exitDone
	}
	how := "fast-forward"
	if l.git(ctx, "merge-base", "--is-ancestor", l.old, target).ok() {
		if r := l.git(ctx, "merge", "--ff-only", target); !r.ok() {
			return l.refuse("the fast-forward to %s failed: %s", short(target), r.why())
		}
	} else {
		if l.subject == "" {
			l.say("land: %s cannot be fast-forwarded to %s; name the merge commit's subject with --subject \"Merge ... into %s\"", branch, l.target, branch)
			return exitCall
		}
		message := l.subject + "\n\n" + strings.Join(trailers, "\n") + "\n"
		if r := l.git(ctx, "merge", "--no-ff", "-m", message, target); !r.ok() {
			conflicted := l.git(ctx, "diff", "--name-only", "--diff-filter=U")
			if !l.git(cleanup(ctx), "merge", "--abort").ok() {
				l.git(cleanup(ctx), "reset", "--hard", l.old)
			}
			if names := strings.Fields(conflicted.stdout); len(names) > 0 {
				return l.refuse("merging %s conflicts in %s; %s is as it was, at %s", l.target, strings.Join(names, ", "), branch, short(l.old))
			}
			return l.refuse("merging %s failed: %s; %s is as it was, at %s", l.target, r.why(), branch, short(l.old))
		}
		how = "merge commit"
	}
	r = l.git(ctx, "rev-parse", "--verify", "HEAD")
	landed := strings.TrimSpace(r.stdout)
	l.say("landed %s into %s: %s, %s..%s", l.target, branch, how, short(l.old), short(landed))

	failed := l.checks(ctx)
	if len(failed) > 0 || l.noPush {
		if !l.git(cleanup(ctx), "reset", "--hard", l.old).ok() {
			l.say("FAIL  could not reset %s to %s: run git reset --hard %s in %s", branch, short(l.old), l.old, l.root)
			l.say("log: %s", l.logPath)
			return exitFailed
		}
		if ctx.Err() != nil {
			l.say("interrupted; %s reset to %s, nothing pushed", branch, short(l.old))
			l.say("log: %s", l.logPath)
			return exitFailed
		}
		if len(failed) > 0 {
			l.say("red: %s; %s reset to %s, nothing pushed", strings.Join(failed, ", "), branch, short(l.old))
			l.say("log: %s", l.logPath)
			return exitFailed
		}
		l.say("green; --no-push: %s reset to %s, nothing pushed or installed", branch, short(l.old))
		l.say("log: %s", l.logPath)
		return exitDone
	}

	if r := l.git(ctx, "push", "origin", branch); !r.ok() {
		if l.git(cleanup(ctx), "reset", "--hard", l.old).ok() {
			l.say("FAIL  git push origin %s: %s; %s reset to %s", branch, r.why(), branch, short(l.old))
		} else {
			l.say("FAIL  git push origin %s: %s; %s is still at %s, unpushed", branch, r.why(), branch, short(landed))
		}
		l.say("log: %s", l.logPath)
		return exitFailed
	}
	l.say("pushed %s %s", branch, short(landed))
	if why := l.installBuild(ctx); why != "" {
		l.say("FAIL  install: %s; %s is pushed, the room's build is the old one", why, branch)
		l.say("log: %s", l.logPath)
		return exitFailed
	}
	l.say("installed %s; the room runs it once restarted", filepath.Join(l.install, "rewake"))
	l.say("log: %s", l.logPath)
	return exitDone
}

// cleanup is the context a rollback runs in: not canceled with the run's,
// since an interrupt during the checks is exactly when v2 must go back, and
// bounded by the runner's own limit (found in review, October 27, 2026).
func cleanup(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

// installBuild builds cmd/rewake into rewake.next beside the room's build
// and renames it over that build, as docs/install.md builds from source: a
// rename does not meet "text file busy" on a running executable, and the
// wrappers already running keep the file they started.
func (l *lander) installBuild(ctx context.Context) string {
	next := filepath.Join(l.install, "rewake.next")
	flags := "-X github.com/praline-labs/rewake/internal/cli.built=" + time.Now().UTC().Format(time.RFC3339)
	r := l.run(ctx, command{dir: l.root, name: "go", args: []string{"build", "-ldflags", flags, "-o", next, "./cmd/rewake"}, limit: buildLimit}, l.log)
	if !r.ok() {
		return "go build: " + r.why()
	}
	if err := os.Rename(next, filepath.Join(l.install, "rewake")); err != nil {
		return err.Error()
	}
	return ""
}

// openLog makes the run's log under .scratch/land, which git ignores.
func (l *lander) openLog() (func(), error) {
	dir := filepath.Join(l.root, ".scratch", "land")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot make the log's directory: %w", err)
	}
	name := time.Now().UTC().Format("20060102T150405Z") + "-" + strings.Map(func(r rune) rune {
		if r == '/' || r == ' ' {
			return '_'
		}
		return r
	}, l.target) + ".log"
	file, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("cannot make the log: %w", err)
	}
	l.log, l.logPath = file, file.Name()
	return func() { _ = file.Close() }, nil
}

// coAuthor is a trailer line of AGENTS.md's "Commits", read rather than
// written here, so the code names no model and follows the file when its
// lines change.
var coAuthor = regexp.MustCompile(`(?m)^Co-Authored-By: .+$`)

// coAuthors reads the trailers every commit ends with; both or none.
func coAuthors(root string) ([]string, error) {
	text, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		return nil, fmt.Errorf("cannot read the Co-Authored-By lines of AGENTS.md: %w", err)
	}
	lines := coAuthor.FindAllString(string(text), -1)
	if len(lines) < 2 {
		return nil, fmt.Errorf("AGENTS.md names %d Co-Authored-By lines, and a commit here ends with both agents'", len(lines))
	}
	return lines, nil
}

// describe is what symbolic-ref said HEAD is.
func describe(r result) string {
	if r.ok() {
		return strings.TrimSpace(r.stdout)
	}
	return "a detached HEAD"
}

func firstLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		return strings.Join(lines[:n], "; ") + fmt.Sprintf(" and %d more", len(lines)-n)
	}
	return strings.Join(lines, "; ")
}

func short(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}
