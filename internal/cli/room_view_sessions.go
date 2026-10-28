package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/core/space"
)

// sessionRowsOf are the sessions as the strip draws them. The telemetry is
// there only when the views carry it (the caller proved to be main's shell),
// and then the shell derives nothing of it: the verdict is rewake's, the
// model is the name the table shows, and the branch is the one the session
// works on.
func sessionRowsOf(dir string, records []space.Record, views []sessionView, mainName string, asMain bool) []SessionRow {
	checkouts := map[string]string{}
	for _, record := range records {
		if !record.State.Final() && record.Checkout.Path != "" && record.Checkout.Branch != "" {
			checkouts[record.Worker.Name+"-"+record.Worker.Harness] = record.Checkout.Branch
		}
	}
	rows := []SessionRow{}
	for _, view := range views {
		row := SessionRow{Name: view.Name, Cwd: view.CWD}
		if asMain && view.Name == mainName {
			row.Own = ptr(true)
		}
		if view.Type != "" {
			row.Type = ptr(view.Type)
		}
		if branch, ok := checkouts[view.Name]; ok {
			row.Branch = ptr(branch)
		} else if branch := branchAt(view.CWD); branch != "" {
			row.Branch = ptr(branch)
		}
		telemetryInto(&row, view.Telemetry)
		if asMain {
			row.Mirror = mirrorOf(dir, view.Name, view.Epoch())
		}
		rows = append(rows, row)
	}
	return rows
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
	if t.Cache != nil {
		row.Cache = &CacheRow{Hit: t.Cache.HitPercent}
		if t.CacheLife != nil && t.CacheLife.ExpiresAt != nil {
			row.Cache.ExpiresAt = ptr(int(t.CacheLife.ExpiresAt.UnixMilli()))
		}
	}
}

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
