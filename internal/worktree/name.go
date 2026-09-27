package worktree

import (
	"regexp"
	"strings"
)

// MaxName bounds a name in bytes: its directory's name and the record's, the
// name with .json after it, fit the 255 bytes a file name may take, and so
// does the .lock git writes beside a branch's last element.
const MaxName = 250

// ValidName says whether a name may name a checkout and its branch: a branch
// name git check-ref-format --branch takes, slashes included, written only in
// ASCII letters, digits, ., _, - and /. The characters are Claude Code's for
// its own worktree names; they leave out +, which stands for the slash in the
// directory's name, so two names never share a directory, and whatever a
// shell would read, so a name goes into the commands a refusal prints as it
// is. Beyond git's rules it is not HEAD or forty hex digits, which git reads
// as something other than a branch, does not end in .json, which would be
// the directory of another name's record, and is at most MaxName long.
func ValidName(name string) bool {
	if !nameCharacters.MatchString(name) || len(name) > MaxName ||
		name == "HEAD" || objectName.MatchString(name) ||
		strings.HasPrefix(name, "-") || strings.HasSuffix(name, ".") ||
		strings.HasSuffix(strings.ToLower(name), ".json") || strings.Contains(name, "..") {
		return false
	}
	for _, element := range strings.Split(name, "/") {
		if element == "" || strings.HasPrefix(element, ".") || strings.HasSuffix(element, ".lock") {
			return false
		}
	}
	return true
}

var (
	nameCharacters = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	objectName     = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
)

// NameRule says what ValidName accepts, for a refusal.
const NameRule = "a branch name git takes, feat/login say, of ASCII letters, digits, ., _, - and /, at most 250 of them; not HEAD or forty hex digits, which git reads as something other than a branch, and not ending in .json"

// flatName is the name as one path element, for the checkout's directory and
// its record: a slash becomes +, which no name holds, as Claude Code names
// the directory of a worktree whose name has a slash. Nested directories
// instead would put a checkout inside another's directory when one name is
// below another, feat and feat/login, and a record among that checkout's
// files.
func flatName(name string) string { return strings.ReplaceAll(name, "/", "+") }
