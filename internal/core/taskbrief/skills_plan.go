package taskbrief

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// The preflight of a brief's skills: the named ones, and the skills their
// markdown links into a sibling folder, which come along so the link leads
// somewhere in the copy (docs/briefs.md#skills).

var (
	// ErrSkillMissing marks a catalog's answer that no skill has the name:
	// a named skill refuses the brief, a linked one is passed over.
	ErrSkillMissing = errors.New("no such skill")
	// ErrSkillBarred marks a catalog's answer that [skills] never bars the
	// name: a named skill refuses the brief, a linked one is noted beside
	// the skill that links it.
	ErrSkillBarred = errors.New("barred from briefs")
)

// lookupError keeps a catalog's own words while errors.Is finds its kind.
type lookupError struct{ kind, err error }

func (e lookupError) Error() string   { return e.err.Error() }
func (e lookupError) Unwrap() []error { return []error{e.kind, e.err} }

// SkillMissing marks err as a catalog's answer that no skill has the name.
func SkillMissing(err error) error { return lookupError{ErrSkillMissing, err} }

// SkillBarred marks err as a catalog's answer that the name is barred.
func SkillBarred(err error) error { return lookupError{ErrSkillBarred, err} }

// pathRun is a run of the characters a relative path is written with; a
// link's target, a path in code or in prose is one.
var pathRun = regexp.MustCompile(`[A-Za-z0-9._~/-]+`)

// planned is a skill found and measured, before anything is written.
type planned struct {
	Skill
	// resolved is the folder with its link resolved, what is copied.
	resolved string
}

// planSkills finds and measures each skill a text names and each skill they
// link, and answers them with the skills.json their snapshot will hold: the
// preflight, which writes nothing. The named come first, in the line's
// order, then the linked as they are found, each once. A text naming none
// needs no catalog.
func planSkills(text string, catalog Catalog) ([]planned, []byte, error) {
	names, err := DeclaredSkills(text)
	if err != nil || len(names) == 0 {
		return nil, nil, err
	}
	if catalog == nil {
		return nil, nil, fmt.Errorf("%w: no skill folders are known here", ErrSkill)
	}
	plan := make([]planned, 0, len(names))
	broughtBy := map[string]string{}
	for _, name := range names {
		from, err := catalog(name)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrSkill, err)
		}
		plan = append(plan, planned{Skill: Skill{Name: name, From: from}})
		broughtBy[name] = ""
	}
	// passed are the linked names the catalog has no skill of, or bars.
	passed := map[string]error{}
	for i := 0; i < len(plan); i++ {
		owner, from := plan[i].Name, plan[i].From
		resolved, err := filepath.EvalSymlinks(from)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s: %v", ErrSkill, from, err)
		}
		var linked []string
		err = measure(resolved, func(file string) error {
			if !strings.EqualFold(filepath.Ext(file), ".md") {
				return nil
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(resolved, filepath.Dir(file))
			if err != nil {
				return err
			}
			linked = appendNew(linked, linksOut(string(raw), filepath.ToSlash(rel))...)
			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s: %v", ErrSkill, owner, err)
		}
		description, err := describe(filepath.Join(resolved, skillFile))
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s: %v", ErrSkill, owner, err)
		}
		plan[i].resolved, plan[i].Description = resolved, description
		for _, name := range linked {
			if _, given := broughtBy[name]; given {
				continue
			}
			lookup, asked := passed[name]
			if !asked {
				var at string
				if at, lookup = catalog(name); lookup == nil {
					if len(plan) == MaxGivenSkills {
						return nil, nil, fmt.Errorf("%w: the skills the brief names and those they link come to more than %d: %s links one past the bound; name fewer, or take the link out of the skill",
							ErrSkill, MaxGivenSkills, strings.Join(chain(broughtBy, owner, name), " → "))
					}
					plan = append(plan, planned{Skill: Skill{Name: name, From: at, BroughtBy: owner}})
					broughtBy[name] = owner
					continue
				}
				passed[name] = lookup
			}
			switch {
			case errors.Is(lookup, ErrSkillBarred):
				plan[i].Barred = append(plan[i].Barred, name)
			case !errors.Is(lookup, ErrSkillMissing):
				return nil, nil, fmt.Errorf("%w: %s, which %s links: %v", ErrSkill, name, owner, lookup)
			}
		}
	}
	listed := make([]Skill, 0, len(plan))
	for _, skill := range plan {
		listed = append(listed, skill.Skill)
	}
	raw, err := encode(listed)
	if err != nil {
		return nil, nil, err
	}
	return plan, raw, nil
}

// chain is the path of links from a named skill to name, which last links.
func chain(broughtBy map[string]string, last, name string) []string {
	links := []string{name}
	for at := last; at != ""; at = broughtBy[at] {
		links = append([]string{at}, links...)
	}
	return links
}

// linksOut answers the sibling folders a markdown file at dir, a slash path
// within its skill, refers to by a relative path: a link's target or a path
// written in the text, "../<name>/…" once resolved against dir. A name
// alone in prose is not followed, nor an absolute path; whether a sibling
// is a skill is the catalog's to say.
func linksOut(text, dir string) []string {
	var names []string
	for _, run := range pathRun.FindAllString(text, -1) {
		if !strings.Contains(run, "../") || strings.HasPrefix(run, "/") {
			continue
		}
		// A sentence may end on the path.
		if strings.HasSuffix(run, ".") && !strings.HasSuffix(run, "..") {
			run = run[:len(run)-1]
		}
		rest, out := strings.CutPrefix(path.Join(dir, run), "../")
		if !out {
			continue
		}
		if name, _, _ := strings.Cut(rest, "/"); skillName.MatchString(name) {
			names = appendNew(names, name)
		}
	}
	return names
}

// appendNew appends the names not in names yet.
func appendNew(names []string, more ...string) []string {
	for _, name := range more {
		if !containsName(names, name) {
			names = append(names, name)
		}
	}
	return names
}
