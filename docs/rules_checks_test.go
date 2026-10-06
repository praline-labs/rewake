package docs

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// rulesWorld is what the check reads apart from the documents, from one module
// root, so the cases can give it a synthetic root read the same way.
type rulesWorld struct {
	root   string    // the module root the named files are relative to
	checks []goTests // the go test commands of AGENTS.md's five checks
	steps  []string  // the steps of the build order
}

// goTests is one go test command of the checks: the tags it builds with,
// whether -race is on, and the packages it tests. A test is proven run only by
// a command that reaches its package and builds its file, never by a tag or a
// mode some other command carries.
type goTests struct {
	command  string
	tags     []string
	race     bool
	patterns []string
}

// readWorld reads the go test commands of AGENTS.md's five checks and the
// steps of docs/v2/stage3.md's build order under root. A command this test
// cannot read for what it runs is a problem, not a command skipped.
func readWorld(root string) (rulesWorld, []string) {
	world := rulesWorld{root: root}
	var problems []string
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		return world, []string{err.Error()}
	}
	world.checks, problems = checksOf(string(agents))
	if len(world.checks) == 0 {
		problems = append(problems, "AGENTS.md's five checks have no go test command, so no named test is run")
	}
	stage, err := os.ReadFile(filepath.Join(root, "docs", "v2", "stage3.md"))
	if err != nil {
		return world, append(problems, err.Error())
	}
	if world.steps, err = buildOrder(string(stage)); err != nil {
		problems = append(problems, "docs/v2/stage3.md: "+err.Error())
	}
	return world, problems
}

// checksOf reads the go test commands of the five checks: the first code block
// of AGENTS.md's Checks section. The blocks after it run the workflow suite and
// other tools, which are not the condition for a commit.
func checksOf(agents string) ([]goTests, []string) {
	var checks []goTests
	var problems []string
	lines, err := mdLines(agents)
	if err != nil {
		return nil, []string{"AGENTS.md: " + err.Error()}
	}
	inChecks, fenced := false, false
	var command strings.Builder
	for _, line := range lines {
		if title, ok := headingTitle(line.text); ok && line.kind == proseLine {
			inChecks = title == "Checks"
			continue
		}
		if !inChecks {
			continue
		}
		if line.kind == fenceLine {
			if fenced {
				break
			}
			fenced = true
			continue
		}
		if line.kind != codeLine {
			continue
		}
		code, _, _ := strings.Cut(line.text, "#")
		code = strings.TrimSpace(code)
		command.WriteString(strings.TrimSuffix(code, "\\") + " ")
		if strings.HasSuffix(code, "\\") {
			continue
		}
		for _, segment := range segments(strings.Fields(command.String())) {
			check, problem, ok := parseGoTest(segment)
			if problem != "" {
				problems = append(problems, problem)
			} else if ok {
				checks = append(checks, check)
			}
		}
		command.Reset()
	}
	return checks, problems
}

// segments splits a command line at the shell's separators.
func segments(words []string) [][]string {
	var all [][]string
	var current []string
	for _, word := range words {
		if slices.Contains([]string{"&&", "||", ";", "|"}, word) {
			all, current = append(all, current), nil
			continue
		}
		current = append(current, word)
	}
	return append(all, current)
}

// testFlags are the go test flags this check reads, and whether each takes a
// value. -race and -tags select files and are kept; -count must leave a
// positive count, since -count=0 runs no test; the rest leave which tests run
// unchanged. Any other flag — -run, -skip, -short, -list — may leave a named
// test out, so a command using it proves nothing and fails.
var testFlags = map[string]bool{
	"race": false, "v": false, "json": false, "failfast": false,
	"tags": true, "count": true, "timeout": true, "p": true, "parallel": true, "shuffle": true, "cpu": true,
}

// parseGoTest reads one segment; ok is false for a segment that runs no go test.
// The one form it reads is go test, optionally behind env -u NAME ...: a prefix
// that sets a variable, GOFLAGS=-count=0 for one, or wraps the command in
// another, can change which tests run in ways the flags after go test do not
// show, so such a command is refused, not read past.
func parseGoTest(words []string) (goTests, string, bool) {
	check := goTests{command: strings.Join(words, " ")}
	unsupported := func(what string) (goTests, string, bool) {
		return goTests{}, fmt.Sprintf("AGENTS.md's check %q %s: docs/rules_test.go reads only go test over directory patterns with flags that run every test", check.command, what), true
	}
	const form = "runs go test other than as go test, optionally behind env -u NAME, so its tests may differ from what its flags say"
	at := slices.Index(words, "go")
	if at < 0 || at+1 == len(words) || words[at+1] != "test" {
		// go -C dir test, or a go binary by another path, still runs tests.
		indirect := at >= 0 && at+1 < len(words) && strings.HasPrefix(words[at+1], "-")
		for i := 1; i < len(words); i++ {
			indirect = indirect || words[i] == "test" && strings.HasSuffix(words[i-1], "go")
		}
		if indirect {
			return unsupported(form)
		}
		return goTests{}, "", false
	}
	prefix := words[:at]
	if len(prefix) > 0 && prefix[0] == "env" {
		prefix = prefix[1:]
		for len(prefix) >= 2 && prefix[0] == "-u" {
			prefix = prefix[2:]
		}
	}
	if len(prefix) > 0 {
		return unsupported(form)
	}
	args := words[at+2:]
	count := "1"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			if arg != "." && arg != "./..." && !strings.HasPrefix(arg, "./") {
				return unsupported("names " + arg + ", which is not a directory pattern of the module")
			}
			check.patterns = append(check.patterns, arg)
			continue
		}
		name, value, inline := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		takes, known := testFlags[name]
		if !known {
			return unsupported("uses -" + name)
		}
		if takes && !inline {
			if i+1 == len(args) {
				return unsupported("ends with -" + name + " and no value")
			}
			i++
			value = args[i]
		}
		// The go command and the test binary each take a flag's last value.
		switch name {
		case "tags":
			check.tags = strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
		case "race":
			if !inline {
				value = "true"
			}
			on, err := strconv.ParseBool(value)
			if err != nil {
				return unsupported("gives -race the value " + value)
			}
			check.race = on
		case "count":
			count = value
		}
	}
	if n, err := strconv.Atoi(count); err != nil || n < 1 {
		return unsupported("runs each test -count=" + count + " times, so not once")
	}
	if len(check.patterns) == 0 {
		check.patterns = []string{"."}
	}
	return check, "", true
}

// reaches answers whether the command tests the package in dir, a path from
// the module root: as go test matches its patterns, where ... skips a
// directory whose name starts with . or _, and testdata.
func (c goTests) reaches(dir string) bool {
	for _, pattern := range c.patterns {
		pattern = path.Clean(strings.TrimPrefix(pattern, "./"))
		prefix, wild := strings.CutSuffix(pattern, "...")
		if !wild {
			if pattern == dir {
				return true
			}
			continue
		}
		prefix = strings.TrimSuffix(prefix, "/")
		var rest string
		switch {
		case prefix == "" || prefix == ".":
			rest = dir
		case dir == prefix:
		case strings.HasPrefix(dir, prefix+"/"):
			rest = strings.TrimPrefix(dir, prefix+"/")
		default:
			continue
		}
		if rest == "." || !slices.ContainsFunc(strings.Split(rest, "/"), func(e string) bool {
			return strings.HasPrefix(e, ".") || strings.HasPrefix(e, "_") || e == "testdata"
		}) {
			return true
		}
	}
	return false
}
