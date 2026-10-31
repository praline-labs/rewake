package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/core/work"
)

// sessionRowsOf are the sessions as the strip draws them. The telemetry is
// there only when the views carry it (the caller proved to be main's shell),
// and then the shell derives nothing of it: the verdict is rewake's, the
// model is the name the table shows, and the path and its branch are where
// the session works now.
// workOf, nil unless asMain, is a run's room work.
func sessionRowsOf(dir string, records []space.Record, views []sessionView, mainName string, workOf func(name, epoch string) string) []SessionRow {
	asMain := workOf != nil
	checkouts := map[string]space.Checkout{}
	for _, record := range records {
		if !record.State.Final() && record.Checkout.Path != "" && record.Checkout.Branch != "" {
			checkouts[record.Worker.Name+"-"+record.Worker.Harness] = record.Checkout
		}
	}
	rows := []SessionRow{}
	for _, view := range views {
		row := SessionRow{Name: view.Name, Cwd: view.CWD, Path: view.CWD}
		if t := view.Telemetry; t != nil && t.Cwd != nil {
			row.Path = *t.Cwd
		}
		if asMain && view.Name == mainName {
			row.Own = ptr(true)
		}
		if view.Type != "" {
			row.Type = ptr(view.Type)
		}
		// The space's record names its checkout's branch; a session that
		// moved elsewhere is read where it is.
		if checkout, ok := checkouts[view.Name]; ok && filepath.Clean(checkout.Path) == filepath.Clean(row.Path) {
			row.Branch = ptr(checkout.Branch)
		} else if branch := branchAt(row.Path); branch != "" {
			row.Branch = ptr(branch)
		}
		telemetryInto(&row, view.Telemetry)
		if asMain {
			row.Mirror = mirrorOf(dir, view.Name, view.Epoch())
			if work := workOf(view.Name, view.Epoch()); work != "" {
				row.Work = &work
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// workReader is the runs' work, with the open briefs listed once for every
// row; a list that does not read leaves the briefs out of every run's work.
func workReader(home taskbrief.Home) func(name, epoch string) string {
	open, _, err := taskbrief.List(home, false)
	if err != nil {
		open = nil
	}
	return func(name, epoch string) string {
		return work.Line(open, name, epoch, work.Awaited(home.Room, name, epoch))
	}
}

// telemetryInto fills the part of a row that comes from the session's
// published state.
func telemetryInto(row *SessionRow, t *session.Snapshot) {
	if t == nil {
		return
	}
	if v := t.Verdict; v != nil {
		row.Verdict = &Verdict{Activity: v.Activity, Label: v.Label, Blocked: v.Blocked, Stale: v.Stale, Mark: Mark(v.Mark)}
	}
	if t.ModelName != nil {
		row.Model = t.ModelName
	} else {
		row.Model = t.Model
	}
	row.Effort, row.Thinking = t.Effort, t.Thinking
	if t.ContextUsed != nil {
		row.Used = ptr(int(*t.ContextUsed))
	}
	if t.ContextWindow != nil {
		row.Window = ptr(int(*t.ContextWindow))
	}
	row.Percent = t.FilledPercent
	for _, limit := range t.RateLimits {
		if window, ok := limitWindows[limit.Kind]; ok {
			shown := LimitRow{Window: window, Percent: limit.Percent}
			if limit.ResetsAt != nil {
				shown.ResetsAt = ptr(int(limit.ResetsAt.UnixMilli()))
			}
			row.Limits = append(row.Limits, shown)
		}
	}
	if t.Cache != nil {
		row.Cache = &CacheRow{Hit: t.Cache.HitPercent}
		if t.CacheLife != nil && t.CacheLife.ExpiresAt != nil {
			row.Cache.ExpiresAt = ptr(int(t.CacheLife.ExpiresAt.UnixMilli()))
		}
	}
}

// limitWindows are the windows the strip shows, by the harness's kind, and
// as it names them; another kind is not shown.
var limitWindows = map[string]string{"five_hour": "5h", "seven_day": "7d"}

// mirrorOf is the session's mirror socket while its run serves one: the
// path is the run's own, so a socket there is this run's.
func mirrorOf(dir, name, epoch string) *string {
	path := registry.MirrorFor(dir, name, epoch)
	if info, err := os.Stat(path); err != nil || info.Mode()&os.ModeSocket == 0 {
		return nil
	}
	return &path
}

func ptr[T any](value T) *T { return &value }

// branchAt is the branch checked out in dir, a detached head's short commit,
// empty outside a repository. It reads HEAD itself rather than ask git: the
// shell asks once a second, and a process per directory each time is what a
// cache in the shell used to avoid.
func branchAt(dir string) string {
	for at := dir; at != "" && at != string(filepath.Separator); at = filepath.Dir(at) {
		entry := filepath.Join(at, ".git")
		info, err := os.Stat(entry)
		if err != nil {
			continue
		}
		gitDir := entry
		if !info.IsDir() {
			raw, err := os.ReadFile(entry)
			if err != nil {
				return ""
			}
			link, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "gitdir:")
			if !ok {
				return ""
			}
			gitDir = strings.TrimSpace(link)
			if !filepath.IsAbs(gitDir) {
				gitDir = filepath.Join(at, gitDir)
			}
		}
		head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
		if err != nil {
			return ""
		}
		text := strings.TrimSpace(string(head))
		if ref, ok := strings.CutPrefix(text, "ref: refs/heads/"); ok {
			return ref
		}
		if ref, ok := strings.CutPrefix(text, "ref: "); ok {
			return ref
		}
		return text[:min(len(text), 7)]
	}
	return ""
}
