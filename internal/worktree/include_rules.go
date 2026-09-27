package worktree

import (
	"regexp"
	"strings"
)

// ignoreRule is one line of a file in .gitignore syntax.
type ignoreRule struct {
	pattern *regexp.Regexp
	negate  bool
	dirOnly bool
	// literal is the pattern up to its first wildcard, for an anchored one:
	// the part of the tree it can name a path in.
	literal string
	glob    bool
}

type ignoreRules []ignoreRule

// parseIgnoreRules reads the lines of a file in .gitignore syntax, as git
// documents it: # starts a comment, ! negates, a slash at the end matches a
// directory only, a slash elsewhere anchors the pattern at the top, * and ?
// and [...] match within one path element, and ** across them as a leading
// **/, a trailing /** or an inner /**/. A backslash quotes the next character.
func parseIgnoreRules(text string) ignoreRules {
	var rules ignoreRules
	for _, line := range strings.Split(text, "\n") {
		line = trimTrailingSpaces(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var rule ignoreRule
		if rest, ok := strings.CutPrefix(line, "!"); ok {
			rule.negate, line = true, rest
		}
		if rest, ok := strings.CutSuffix(line, "/"); ok && !strings.HasSuffix(rest, "\\") {
			rule.dirOnly, line = true, rest
		}
		anchored := strings.Contains(line, "/")
		line = strings.TrimPrefix(line, "/")
		if line == "" {
			continue
		}
		body := globRegexp(line)
		if anchored {
			rule.pattern = regexp.MustCompile("^" + body + "$")
			rule.literal = line
			if at := strings.IndexAny(line, `*?[\`); at >= 0 {
				rule.literal, rule.glob = line[:at], true
			}
		} else {
			rule.pattern = regexp.MustCompile("^(?:.*/)?" + body + "$")
		}
		rules = append(rules, rule)
	}
	return rules
}

// trimTrailingSpaces drops the spaces ending a line, except one quoted by a
// backslash.
func trimTrailingSpaces(line string) string {
	for strings.HasSuffix(line, " ") && !strings.HasSuffix(line, `\ `) {
		line = line[:len(line)-1]
	}
	return line
}

// globRegexp turns a pattern into the body of a regular expression over a
// slash-separated path.
func globRegexp(glob string) string {
	var out strings.Builder
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; {
		case c == '*' && strings.HasPrefix(glob[i:], "**") && (i == 0 || glob[i-1] == '/') && (i+2 == len(glob) || glob[i+2] == '/'):
			if i+2 == len(glob) {
				// ** alone, or a/** — everything inside a, which the slash
				// before it already required.
				out.WriteString(".*")
			} else {
				// **/ at the start or /**/ within: any number of
				// directories, none included.
				out.WriteString("(?:[^/]*/)*")
				i++
			}
			i++
		case c == '*':
			out.WriteString("[^/]*")
		case c == '?':
			out.WriteString("[^/]")
		case c == '[':
			class, width := globClass(glob[i:])
			if width == 0 {
				out.WriteString(`\[`)
				continue
			}
			out.WriteString(class)
			i += width - 1
		case c == '\\' && i+1 < len(glob):
			i++
			out.WriteString(regexp.QuoteMeta(glob[i : i+1]))
		default:
			out.WriteString(regexp.QuoteMeta(glob[i : i+1]))
		}
	}
	return out.String()
}

// globClass reads a bracket expression at the start of glob and returns it as
// a regular expression class and the width it took, or a width of 0 when the
// bracket is not closed and stands for itself.
func globClass(glob string) (string, int) {
	i := 1
	negate := false
	if i < len(glob) && (glob[i] == '!' || glob[i] == '^') {
		negate, i = true, i+1
	}
	start := i
	if i < len(glob) && glob[i] == ']' {
		i++
	}
	for i < len(glob) && glob[i] != ']' {
		i++
	}
	if i >= len(glob) {
		return "", 0
	}
	var class strings.Builder
	class.WriteString("[")
	if negate {
		class.WriteString("^/")
	}
	for _, r := range glob[start:i] {
		if r == '\\' || r == '[' || r == ']' || r == '^' {
			class.WriteString(`\`)
		}
		class.WriteRune(r)
	}
	class.WriteString("]")
	return class.String(), i + 1
}

// matches says whether the rules take in a path, relative to the top: the
// last rule matching it decides, and a path in a directory they take in is
// taken in whatever a later rule says of the path itself, as git has it.
func (rules ignoreRules) matches(path string, isDir bool) bool {
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		if rules.decide(strings.Join(parts[:i], "/"), true) {
			return true
		}
	}
	return rules.decide(path, isDir)
}

func (rules ignoreRules) decide(path string, isDir bool) bool {
	matched := false
	for _, rule := range rules {
		if (!rule.dirOnly || isDir) && rule.pattern.MatchString(path) {
			matched = !rule.negate
		}
	}
	return matched
}

// opens says whether a wholly ignored directory is worth listing: a pattern
// names a path in it, or it itself. A pattern that may match at any depth
// does not open it, as in Claude Code: it matches files in tracked
// directories, which are listed one by one already, and opening every ignored
// directory for it would walk a node_modules.
func (rules ignoreRules) opens(dir string) bool {
	if rules.matches(dir, true) {
		return true
	}
	for _, rule := range rules {
		if rule.literal == "" || rule.negate {
			continue
		}
		if strings.HasPrefix(rule.literal, dir+"/") || rule.glob && strings.HasPrefix(dir+"/", rule.literal) {
			return true
		}
	}
	return false
}
