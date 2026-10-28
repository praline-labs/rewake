package taskbrief

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A brief's skills (docs/briefs.md#skills). Main names them in one line of
// the text, "Skills: mods, mp-tdd"; rewake copies each from the folders
// main's harness reads into the brief, one snapshot per edition, and the
// worker reads the copies by the paths its letter gives. The edition the
// log's last edit commits selects its snapshot, so a text and its skills
// change together: an edit cut short leaves the previous edition's whole.

const (
	// skillsDir holds the snapshots: skills/<edition>/<name>/ and
	// skills/<edition>/skills.json.
	skillsDir = "skills"
	// skillsFile lists a snapshot's skills.
	skillsFile = "skills.json"
	// skillFile is a skill's own file, which names and describes it.
	skillFile = "SKILL.md"
	// MaxSkills bounds the skills one brief names.
	MaxSkills = 8
	// MaxGivenSkills bounds the skills one brief gives: those it names and
	// those they link, which come along.
	MaxGivenSkills = 16
	// maxSkillBytes and maxSkillEntries bound one skill's copy: about three
	// times the largest skill seen in real work, 5 MB in 1,695 files.
	maxSkillBytes   = 16 << 20
	maxSkillEntries = 4096
	// maxDescription cuts a skill's description, in code points, as the
	// harnesses bound theirs.
	maxDescription = 1024
)

var (
	// skillsLine declares the skills, at the start of a line.
	skillsLine = regexp.MustCompile(`^Skills:(.*)$`)
	// skillName is a name a skill folder may have and a path cannot escape.
	skillName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
)

// ErrSkill is a skill a brief names that cannot be given: missing, barred,
// past its bound, or holding what a copy cannot take. Nothing is written.
var ErrSkill = errors.New("a skill the brief names cannot be given")

// Catalog finds a skill by name where main's harness reads skills, and
// answers its folder; the caller knows the harness and the never-list.
type Catalog func(name string) (string, error)

// Skill is one entry of a snapshot's skills.json.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// From is the folder it was copied from.
	From string `json:"from"`
	// BroughtBy names the skill whose link brought this one; none for a
	// skill the text names.
	BroughtBy string `json:"broughtBy,omitempty"`
	// Barred are the skills this one links that [skills] never bars: not
	// given, so those links lead nowhere in the copy.
	Barred []string `json:"barred,omitempty"`
}

// SkillCopy is a skill as a reader is given it: its SKILL.md in the copy.
type SkillCopy struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Path        string   `json:"path"`
	BroughtBy   string   `json:"broughtBy,omitempty"`
	Barred      []string `json:"barred,omitempty"`
}

// DeclaredSkills reads the names a text's Skills line gives, none without
// one. A second line, a name no skill folder may have, a plugin's skill, a
// repeat or more than MaxSkills is ErrBadText.
func DeclaredSkills(text string) ([]string, error) {
	var names []string
	declared := false
	fence := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if m := fenceLine.FindStringSubmatch(line); m != nil {
			switch {
			case fence == "":
				fence = m[1]
			case m[1][0] == fence[0] && len(m[1]) >= len(fence) && strings.TrimSpace(m[2]) == "":
				fence = ""
			}
			continue
		}
		m := skillsLine.FindStringSubmatch(line)
		if fence != "" || m == nil {
			continue
		}
		if declared {
			return nil, fmt.Errorf("%w: two Skills lines; name every skill in one, Skills: a, b", ErrBadText)
		}
		declared = true
		for _, part := range strings.Split(m[1], ",") {
			name := strings.TrimSpace(part)
			switch {
			case strings.Contains(name, ":"):
				return nil, fmt.Errorf("%w: %q is a plugin's skill, which a brief does not give; link the skill's folder into main's user skill folder and name it by the folder", ErrBadText, name)
			case !skillName.MatchString(name):
				return nil, fmt.Errorf("%w: %q in the Skills line is no skill's name: lower-case letters, digits, dot, dash and underscore, at most 64", ErrBadText, name)
			case containsName(names, name):
				return nil, fmt.Errorf("%w: the Skills line names %s twice", ErrBadText, name)
			}
			names = append(names, name)
		}
	}
	if len(names) > MaxSkills {
		return nil, fmt.Errorf("%w: the Skills line names %d skills, past the bound of %d", ErrBadText, len(names), MaxSkills)
	}
	return names, nil
}

func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// snapshotPath is the snapshot of edition in the brief at path.
func snapshotPath(path string, edition int) string {
	return filepath.Join(path, skillsDir, FormatID(edition))
}

// Skills are the copies of the current edition's skills, none for a text
// that names none; an error says the snapshot could not be read. A closed
// brief has none: its copies went with the close.
func (b Brief) Skills() ([]SkillCopy, error) {
	if b.path == "" || b.IsClosed() {
		return nil, nil
	}
	snapshot := snapshotPath(b.path, b.Edition)
	raw, err := state.ReadRegular(filepath.Join(snapshot, skillsFile), maxRecordBytes)
	if errors.Is(err, os.ErrNotExist) {
		if names, _ := DeclaredSkills(b.Text); len(names) > 0 {
			return nil, fmt.Errorf("the copies of edition %d are missing", b.Edition)
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var listed []Skill
	if err := json.Unmarshal(raw, &listed); err != nil {
		return nil, fmt.Errorf("%s does not read: %w", filepath.Join(snapshot, skillsFile), err)
	}
	copies := make([]SkillCopy, 0, len(listed))
	for _, skill := range listed {
		if !skillName.MatchString(skill.Name) {
			return nil, fmt.Errorf("%s names %q, no skill's name", filepath.Join(snapshot, skillsFile), skill.Name)
		}
		copies = append(copies, SkillCopy{
			Name: skill.Name, Description: skill.Description, Path: filepath.Join(snapshot, skill.Name, skillFile),
			BroughtBy: skill.BroughtBy, Barred: skill.Barred,
		})
	}
	return copies, nil
}

// SkillNames are the names the current text's Skills line gives, what a
// list shows; none where the line does not read.
func (b Brief) SkillNames() []string {
	names, _ := DeclaredSkills(b.Text)
	return names
}

// measure walks a skill's folder as a copy will, counting its entries and
// bytes against the bound, and hands each file within the bound to visit. A
// link to a regular file counts as that file; any other link, a device or a
// socket is refused, named.
func measure(root string, visit func(path string) error) error {
	entries, size := 0, int64(0)
	var walk func(dir string) error
	walk = func(dir string) error {
		listed, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range listed {
			path := filepath.Join(dir, entry.Name())
			info, err := regularOrDir(path)
			if err != nil {
				return err
			}
			if entries++; entries > maxSkillEntries {
				return fmt.Errorf("more than %d entries, past the bound of a skill's copy", maxSkillEntries)
			}
			if info.IsDir() {
				if err := walk(path); err != nil {
					return err
				}
				continue
			}
			if size += info.Size(); size > maxSkillBytes {
				return fmt.Errorf("more than %d bytes, past the bound of a skill's copy", maxSkillBytes)
			}
			if err := visit(path); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

// regularOrDir is what a copy takes at path: a folder, a regular file, or a
// link to a regular file, read as that file. A link to a folder is refused:
// following it could leave the skill or loop.
func regularOrDir(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	switch mode := info.Mode(); {
	case mode.IsDir(), mode.IsRegular():
		return info, nil
	case mode&os.ModeSymlink != 0:
		target, err := os.Stat(path)
		if err != nil || !target.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is a link to no regular file; a copy takes files and folders", path)
		}
		return target, nil
	}
	return nil, fmt.Errorf("%s is neither a file nor a folder", path)
}

// describe reads the description in a SKILL.md's front matter: one line,
// quoted or not, or a folded block of indented lines, cut to
// maxDescription. A SKILL.md without one is described by nothing.
func describe(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("no %s: %w", skillFile, err)
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 16<<10)
	n, _ := file.Read(head)
	lines := strings.Split(strings.ReplaceAll(string(head[:n]), "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", nil
	}
	for i := 1; i < len(lines) && strings.TrimSpace(lines[i]) != "---"; i++ {
		value, ok := strings.CutPrefix(lines[i], "description:")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" || strings.ContainsAny(value[:1], ">|") {
			var block []string
			for _, next := range lines[i+1:] {
				if next == "" || (next[0] != ' ' && next[0] != '\t') {
					break
				}
				block = append(block, strings.TrimSpace(next))
			}
			value = strings.Join(block, " ")
		} else if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		return cutDescription(value), nil
	}
	return "", nil
}

func cutDescription(text string) string {
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "?")
	}
	runes := []rune(text)
	if len(runes) <= maxDescription {
		return text
	}
	return string(runes[:maxDescription-1]) + "…"
}
