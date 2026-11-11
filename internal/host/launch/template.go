package launch

// Turning one alias into what space open starts: its harness, role, arguments
// and applied settings, refused where it would start something other than what
// it says. A template an obsolete setting left out is resolved the same way
// before it is omitted, so an old type never screens a malformed setting
// (alias_obsolete.go).

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// Template is an alias resolved against its harness: what space open starts.
type Template struct {
	Name        string
	Harness     adapter.Harness
	Role        role.Role
	Command     string
	Description string
	// Args are the alias's arguments with its applied settings in them.
	Args     []string
	Settings AliasSettings
	// Worktree says the worker starts in a new worktree of its own.
	Worktree bool
	// EnterWorktree says it may enter one mid-session instead.
	EnterWorktree bool
	// Applied says which of the settings became flags.
	Applied struct{ Model, Effort, Context bool }
}

// MainOnlyTemplateError names an old worker alias whose harness now launches
// main only. Listings omit it without rejecting the other templates.
type MainOnlyTemplateError struct {
	Alias, Harness string
}

func (e *MainOnlyTemplateError) Error() string {
	return fmt.Sprintf("alias %q uses %s, whose harness launches main only; choose a worker template from rewake workers", e.Alias, e.Harness)
}

// aliasDef is one definition of a template, with where it was read.
type aliasDef struct {
	value   Alias
	source  string
	path    string
	project bool
}

// obsoleteDef is a definition an obsolete setting left out, with the migration
// notice that says what to fix.
type obsoleteDef struct {
	aliasDef
	notice string
}

// OmittedTemplateError names an alias an obsolete setting left unavailable,
// with the migration notice that says what to fix. A listing checks the
// alias's current settings and then leaves it out, as it does a main-only one.
type OmittedTemplateError struct {
	Alias, Notice string
}

func (e *OmittedTemplateError) Error() string {
	return fmt.Sprintf("the alias %q is unavailable until its obsolete setting is fixed: %s", e.Alias, e.Notice)
}

// OmittedNames are the templates an obsolete definition left unavailable, in
// order, for a listing that checks their current settings before leaving them
// out.
func (s *Set) OmittedNames() []string {
	names := make([]string, 0, len(s.omitted))
	for name := range s.omitted {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Template resolves one alias, refusing one that would start something other
// than what it says.
func (s *Set) Template(name string, catalog adapter.Catalog) (Template, error) {
	if def, omitted := s.omitted[name]; omitted {
		// Its current settings are checked as any other's: an obsolete type
		// may leave a valid template out, never screen a malformed one.
		if _, err := s.resolve(name, def.aliasDef, catalog); err != nil {
			return Template{}, err
		}
		return Template{}, &OmittedTemplateError{Alias: name, Notice: def.notice}
	}
	if where, off := s.aliasOff[name]; off {
		return Template{}, fmt.Errorf("the alias %q is turned off in %s ([aliases] off); %s", name, where, s.available())
	}
	value, source, ok := s.Lookup(name)
	if !ok {
		return Template{}, fmt.Errorf("no alias %q; %s", name, s.available())
	}
	return s.resolve(name, aliasDef{value: value, source: source, path: s.paths[name], project: s.fromProject[name]}, catalog)
}

// resolve turns one definition into what space open starts, refusing one that
// would start something other than what it says.
func (s *Set) resolve(name string, def aliasDef, catalog adapter.Catalog) (Template, error) {
	value, source := def.value, def.source
	if err := value.check(name, source, catalog.IDs(), def.project); err != nil {
		return Template{}, err
	}
	h, _ := catalog.Find(value.Harness)
	if h.Offers().MainOnly {
		return Template{}, &MainOnlyTemplateError{Alias: name, Harness: h.Title()}
	}
	part := role.General
	if value.Role != "" {
		part, _ = role.Find(value.Role)
	}
	template := Template{
		Name: name, Harness: h, Role: part, Command: value.Command,
		Description: value.Description, Args: slices.Clone(value.Args), Settings: value.Launch,
		Worktree: value.Worktree, EnterWorktree: value.EnterWorktree,
	}
	if err := worktreeField(h, value); err != nil {
		return Template{}, fmt.Errorf("alias %q in %s: %w", name, source, err)
	}
	if err := enterWorktreeField(h, value); err != nil {
		return Template{}, fmt.Errorf("alias %q in %s: %w", name, source, err)
	}
	flags, err := settingFlags(h, value.Launch, &template)
	if err != nil {
		return Template{}, fmt.Errorf("alias %q: %w", name, err)
	}
	if len(flags) > 0 {
		// Structured fields replace an older copy in args, even when args
		// ends with a prompt terminator. New flags still precede that tail.
		visible := adapter.BeforeTerminator(template.Args)
		kept := replaced(visible, flags, h.SingleUseFlags())
		template.Args = adapter.AddFlags(append(slices.Clone(kept), template.Args[len(visible):]...), flags...)
	}
	return template, nil
}

// worktreeField refuses worktree = true where it cannot be what it says: on a
// harness rewake makes no worktree for, or beside the harness's worktree flag
// in args, which would leave two answers to one question.
func worktreeField(h adapter.Harness, value Alias) error {
	if !value.Worktree {
		return nil
	}
	taker, ok := h.(adapter.WorktreeHarness)
	if !ok {
		return fmt.Errorf("worktree = true, yet %s makes no worktree through rewake", h.Title())
	}
	if adapter.HasFlag(adapter.BeforeTerminator(value.Args), taker.WorktreeFlag()) {
		return fmt.Errorf("worktree = true and %s in args; keep one, and name a worktree with --worktree=<name> on space open", taker.WorktreeFlag())
	}
	return nil
}

// enterWorktreeField refuses enter_worktree = true on a harness whose session
// cannot move into a checkout, and beside a checkout made at the start, by
// worktree = true or the harness's flag in args: a worker has one checkout,
// made at its start or entered later, not both.
func enterWorktreeField(h adapter.Harness, value Alias) error {
	if !value.EnterWorktree {
		return nil
	}
	if _, ok := h.(adapter.WorktreeEntryHarness); !ok {
		return fmt.Errorf("enter_worktree = true, yet a %s session cannot enter a worktree mid-session; worktree = true starts it in one", h.Title())
	}
	if value.Worktree {
		return errors.New("worktree = true and enter_worktree = true; a worker has one checkout, made at its start or entered later, so keep one")
	}
	if taker, ok := h.(adapter.WorktreeHarness); ok && adapter.HasFlag(adapter.BeforeTerminator(value.Args), taker.WorktreeFlag()) {
		return fmt.Errorf("enter_worktree = true and %s in args; a worker has one checkout, made at its start or entered later, so keep one", taker.WorktreeFlag())
	}
	return nil
}

// settingFlags turns the settings into the harness's flags and records which
// of them it applied. Context is only shown on a harness with no flag for it.
func settingFlags(h adapter.Harness, settings AliasSettings, template *Template) ([]string, error) {
	translator, ok := h.(adapter.LaunchSettingsHarness)
	if settings.Model == "" && settings.Effort == "" && (!ok || settings.Context == "" || !translator.AppliesContext()) {
		return nil, nil
	}
	if !ok {
		return nil, fmt.Errorf("%s has no launch settings; put the model and effort in args", h.Title())
	}
	context := settings.Context
	if !translator.AppliesContext() {
		context = ""
	}
	flags, err := translator.LaunchSettings(settings.Model, settings.Effort, context)
	if err != nil {
		return nil, err
	}
	template.Applied.Model, template.Applied.Effort, template.Applied.Context = settings.Model != "", settings.Effort != "", context != ""
	return flags, nil
}

// check refuses an alias that would start something unusable, and says what.
// A worker that fails three layers down, complaining about a flag the person
// never typed, is worse than no alias at all.
func (a Alias) check(name, source string, knownHarnesses []string, fromProject bool) error {
	for _, value := range []string{a.Description, a.Launch.Model, a.Launch.Effort, a.Launch.Context, a.Launch.Label} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("alias %q in %s has a multiline description or launch setting; each must be one line", name, source)
		}
	}
	if a.Harness == "" {
		return fmt.Errorf("alias %q in %s names no harness; add harness = \"<name>\"", name, source)
	}
	if !slices.Contains(knownHarnesses, a.Harness) {
		return fmt.Errorf("alias %q in %s starts %q, which is not a harness; known: %s",
			name, source, a.Harness, strings.Join(knownHarnesses, ", "))
	}
	if a.Role != "" {
		if part, found := role.Find(a.Role); !found || part.Spaces {
			return fmt.Errorf("alias %q in %s has role = %q; a worker's role is %q or %q", name, source, a.Role, role.Write.ID, role.General.ID)
		}
	}
	if fromProject && (a.Role != "" || a.Command != "") {
		// A file in whatever directory somebody happens to be in must not
		// choose the role of a worker opened there, nor the program it starts:
		// the program is whatever runs with the person's credentials.
		what := "role"
		if a.Command != "" {
			what = "command"
		}
		return fmt.Errorf("alias %q in %s sets %s; it belongs in the user alias file, not in one carried by a directory", name, source, what)
	}
	for _, argument := range a.Args {
		if argument == "" {
			return fmt.Errorf("alias %q in %s has an empty argument", name, source)
		}
	}
	return nil
}
