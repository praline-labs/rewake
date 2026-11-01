package update

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// ChangelogFile is the changelog, at the repository's root and in the
// entry package of each release.
const ChangelogFile = "CHANGELOG.md"

// Release is one release's entry: its changes by part, in the file's order.
type Release struct {
	Version string   `json:"version"`
	Parts   []Change `json:"parts"`
}

// Change is what one release changed in one part: a part of the manifest,
// or docs.
type Change struct {
	Part  string   `json:"part"`
	Lines []string `json:"lines"`
}

// ParseChangelog reads the changelog: "## <version>" for each release,
// newest first, "### <part>" under it, and "- " lines under a part, a line
// indented under one continuing it. Text before the first release is its
// preface; anything else out of place fails, so a release cannot ship an
// entry the command would show half of.
func ParseChangelog(data []byte) ([]Release, error) {
	var out []Release
	scanner := bufio.NewScanner(bytes.NewReader(data))
	n := 0
	for scanner.Scan() {
		n++
		line := strings.TrimRight(scanner.Text(), " \t")
		bad := func(why string) error { return fmt.Errorf("%s line %d: %s: %q", ChangelogFile, n, why, line) }
		switch {
		case strings.HasPrefix(line, "## "):
			version := strings.TrimSpace(line[3:])
			v, err := ParseVersion(version)
			if err != nil {
				return nil, bad(err.Error())
			}
			if len(out) > 0 {
				last, _ := ParseVersion(out[len(out)-1].Version)
				if v.Compare(last) >= 0 {
					return nil, bad("releases go newest first")
				}
			}
			out = append(out, Release{Version: v.String()})
		case len(out) == 0:
			// The preface.
		case strings.HasPrefix(line, "### "):
			out[len(out)-1].Parts = append(out[len(out)-1].Parts, Change{Part: strings.TrimSpace(line[4:])})
		case line == "":
		case strings.HasPrefix(line, "- "):
			r := &out[len(out)-1]
			if len(r.Parts) == 0 {
				return nil, bad("a change before its part")
			}
			c := &r.Parts[len(r.Parts)-1]
			c.Lines = append(c.Lines, strings.TrimSpace(line[2:]))
		case strings.HasPrefix(line, "  "):
			r := &out[len(out)-1]
			if len(r.Parts) == 0 || len(r.Parts[len(r.Parts)-1].Lines) == 0 {
				return nil, bad("a continued line with no change before it")
			}
			lines := r.Parts[len(r.Parts)-1].Lines
			lines[len(lines)-1] += " " + strings.TrimSpace(line)
		default:
			return nil, bad("not a release, a part or a change")
		}
	}
	return out, scanner.Err()
}

// Between is the releases newer than from and no newer than to, newest
// first: what installing to over from brings.
func Between(releases []Release, from, to string) []Release {
	var out []Release
	for _, r := range releases {
		if Newer(r.Version, from) && !Newer(r.Version, to) {
			out = append(out, r)
		}
	}
	return out
}

// Find is the entry of version.
func Find(releases []Release, version string) (Release, bool) {
	for _, r := range releases {
		if r.Version == version {
			return r, true
		}
	}
	return Release{}, false
}

// Lines is a release's entry as the command and the release gate print it.
func (r Release) Lines() []string {
	lines := []string{r.Version}
	for _, c := range r.Parts {
		lines = append(lines, "  "+c.Part)
		for _, l := range c.Lines {
			lines = append(lines, "    - "+l)
		}
	}
	return lines
}
