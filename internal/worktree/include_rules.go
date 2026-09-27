package worktree

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"slices"
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
//
// The file is the repository's content, a stranger's as well, so a line that
// makes no expression — a range running backwards, a class of no known name —
// is named in the second result and left out, and the other lines apply.
func parseIgnoreRules(text string) (ignoreRules, []string) {
	var rules ignoreRules
	var unread []string
	for _, line := range strings.Split(text, "\n") {
		line = trimTrailingSpaces(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		written := line
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
		body, err := globRegexp(line)
		expression := "^(?:.*/)?" + body + "$"
		if anchored {
			expression = "^" + body + "$"
			rule.literal = line
			if at := strings.IndexAny(line, `*?[\`); at >= 0 {
				rule.literal, rule.glob = line[:at], true
			}
		}
		var pattern *regexp.Regexp
		if err == nil {
			pattern, err = regexp.Compile(expression)
		}
		if err != nil {
			unread = append(unread, fmt.Sprintf("%s: the line %q is not a pattern rewake can read, so it copies nothing: %v", IncludeFile, written, err))
			continue
		}
		rule.pattern = pattern
		rules = append(rules, rule)
	}
	return rules, unread
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
func globRegexp(glob string) (string, error) {
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
			class, width, err := globClass(glob[i:])
			if err != nil {
				return "", err
			}
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
	return out.String(), nil
}

// globClass reads a bracket expression at the start of glob and returns it as
// a regular expression class and the width it took, or a width of 0 when the
// bracket is not closed and stands for itself. Within it a backslash quotes the
// next character and [:name:] is one of git's twelve named classes, as in its
// wildmatch; another name, which the expression syntax might know — word — and
// git does not, and a range running backwards, are an error. As in git, the
// class never matches a slash, whatever its range: a[.-0]b does not match a/b.
func globClass(glob string) (string, int, error) {
	i := 1
	negate := false
	if i < len(glob) && (glob[i] == '!' || glob[i] == '^') {
		negate, i = true, i+1
	}
	var class strings.Builder
	class.WriteString("[")
	if negate {
		class.WriteString("^")
	}
	for first := true; ; first = false {
		if i >= len(glob) {
			return "", 0, nil
		}
		c := glob[i]
		switch {
		case c == ']' && !first:
			class.WriteString("]")
			text, err := withoutSlash(class.String())
			return text, i + 1, err
		case c == '\\' && i+1 < len(glob):
			class.WriteString(regexp.QuoteMeta(glob[i+1 : i+2]))
			i += 2
		case c == '[' && strings.HasPrefix(glob[i:], "[:"):
			end := strings.Index(glob[i+2:], ":]")
			if end < 0 {
				class.WriteString(`\[`)
				i++
				continue
			}
			if name := glob[i+2 : i+2+end]; !slices.Contains(gitClasses, name) {
				return "", 0, fmt.Errorf("[:%s:] is not a class git knows", name)
			}
			class.WriteString(glob[i : i+2+end+2])
			i += 2 + end + 2
		case c == '\\' || c == '[' || c == ']' || c == '^':
			class.WriteString(`\`)
			class.WriteByte(c)
			i++
		default:
			class.WriteByte(c)
			i++
		}
	}
}

// gitClasses are the names wildmatch knows inside brackets.
var gitClasses = []string{"alnum", "alpha", "blank", "cntrl", "digit", "graph", "lower", "print", "punct", "space", "upper", "xdigit"}

// withoutSlash takes the slash out of a bracket expression's set, so neither a
// range nor a negation spanning it matches one.
func withoutSlash(class string) (string, error) {
	parsed, err := syntax.Parse(class, syntax.Perl)
	if err != nil {
		return "", err
	}
	var ranges []rune
	switch parsed.Op {
	case syntax.OpCharClass:
		ranges = parsed.Rune
	case syntax.OpLiteral:
		for _, r := range parsed.Rune {
			ranges = append(ranges, r, r)
		}
	default:
		return "", fmt.Errorf("%s is not a set of characters", class)
	}
	var kept []rune
	for j := 0; j+1 < len(ranges); j += 2 {
		lo, hi := ranges[j], ranges[j+1]
		if lo <= '/' && '/' <= hi {
			if lo < '/' {
				kept = append(kept, lo, '/'-1)
			}
			if hi > '/' {
				kept = append(kept, '/'+1, hi)
			}
			continue
		}
		kept = append(kept, lo, hi)
	}
	set := &syntax.Regexp{Op: syntax.OpCharClass, Rune: kept}
	return set.String(), nil
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
