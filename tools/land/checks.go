package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	raceLimit     = 25 * time.Minute
	workflowLimit = 12 * time.Minute
	shellLimit    = 10 * time.Minute
)

// checks runs the checks of a step's last commit (AGENTS.md, "Checks") on
// the merged v2, every one even after a red one, so one run names every
// failure, and answers the names of those that failed.
func (l *lander) checks(ctx context.Context) []string {
	var failed []string
	check := func(name string, commands ...command) {
		started := time.Now()
		for _, c := range commands {
			r := l.run(ctx, c, l.log)
			if !r.ok() {
				l.say("FAIL  %-8s %s (%s)", name, failures(r), time.Since(started).Round(time.Second))
				failed = append(failed, name)
				return
			}
		}
		l.say("ok    %-8s %s", name, time.Since(started).Round(time.Second))
	}
	parallel := 4
	if heavy(l.processes()) {
		parallel = 2
	}
	check("race", command{
		dir: l.root, name: "go", limit: raceLimit,
		args: []string{"test", "-race", "-shuffle=on", "-timeout", "20m", "-p", strconv.Itoa(parallel), "./..."},
	})
	check("workflow", command{
		dir: l.root, name: "go", env: []string{"REWAKE_WORKFLOW=1"}, limit: workflowLimit,
		args: []string{"test", "-count=1", "-timeout", "10m", "-v", "./test/workflow/..."},
	})
	if r := l.git(ctx, "diff", "--quiet", l.old, "HEAD", "--", "shell/"); r.err == nil && r.code == 0 {
		l.say("skip  shell    shell/ unchanged")
	} else {
		shell := filepath.Join(l.root, "shell")
		check("shell",
			command{dir: shell, name: "bun", args: []string{"install", "--frozen-lockfile"}, limit: shellLimit},
			command{dir: shell, name: "bun", args: []string{"run", "check"}, limit: shellLimit})
	}
	return failed
}

// failLine is a line of go test's output that names what failed.
var failLine = regexp.MustCompile(`^(\s*--- FAIL: \S+|FAIL\s+\S+|panic: .*|WARNING: DATA RACE)`)

// failures is the shortest account of a red check: the first lines naming
// what failed, else the end of what the command said.
func failures(r result) string {
	var named []string
	for _, line := range strings.Split(r.stdout+"\n"+r.stderr, "\n") {
		if match := failLine.FindString(line); match != "" && !slices.Contains(named, strings.TrimSpace(match)) {
			named = append(named, strings.TrimSpace(match))
		}
	}
	if len(named) == 0 {
		return r.why()
	}
	if len(named) > 6 {
		named = append(named[:6], "…")
	}
	return strings.Join(named, "; ")
}

// heavy says whether the process list shows another heavy run: a go test
// with -race, or one over a whole tree (./...). AGENTS.md has a run take -p 2
// beside one and -p 4 otherwise.
func heavy(commands [][]string) bool {
	for _, argv := range commands {
		if len(argv) < 2 || filepath.Base(argv[0]) != "go" || argv[1] != "test" {
			continue
		}
		for _, arg := range argv[2:] {
			if arg == "-race" || strings.HasSuffix(arg, "/...") {
				return true
			}
		}
	}
	return false
}

// processes reads every process's command line from /proc; one that ended
// or cannot be read is left out.
func processes() [][]string {
	entries, _ := os.ReadDir("/proc")
	var commands [][]string
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || len(raw) == 0 {
			continue
		}
		commands = append(commands, strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00"))
	}
	return commands
}
