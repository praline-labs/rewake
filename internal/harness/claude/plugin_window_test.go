package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/harness/claude/telemetry"
)

// The plugin reads the auto-compact window where the harness does — the
// variable through $.env, the key through $.settings — at the session's start
// and at every measure of the context, and sends of the settings nothing but
// that key: they can hold a person's secrets in their env block.
func TestThePluginReportsTheAutoCompactWindow(t *testing.T) {
	dir := t.TempDir()
	argv := []string{"/bin/rewake", "observe", "/run/s.obs"}
	if err := writePlugin(filepath.Join(dir, "plugin"), argv, ""); err != nil {
		t.Fatal(err)
	}
	module, err := os.ReadFile(filepath.Join(dir, "plugin", "hooks", "rewake.js"))
	if err != nil {
		t.Fatal(err)
	}
	world := hostWorld{
		Env: map[string]string{"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "300000", "OTHER": "SECRET-ENV"},
		Settings: map[string]any{
			"autoCompactWindow": 400000,
			"env":               map[string]any{"TOKEN": "SECRET-SETTING"},
		},
	}
	calls, refused := runModuleIn(t, module, world, [][2]any{
		{"session.start", map[string]any{"cwd": "/w", "surface": "terminal", "isInteractive": true}},
		{"session.measure", map[string]any{"context": map[string]any{"tokens": 150000, "window": 1000000, "percent": 15}, "changed": []any{"context"}}},
	})
	if len(refused) != 0 || len(calls) != 2 {
		t.Fatalf("ran %d, refused %v", len(calls), refused)
	}
	limit := map[string]any{"env": "300000", "settings": 400000.0}
	for index, call := range calls {
		if strings.Contains(call.Init.Stdin, "SECRET") {
			t.Errorf("run %d carries more of the environment or settings than the window: %s", index, call.Init.Stdin)
		}
		var sent map[string]any
		if err := json.Unmarshal([]byte(call.Init.Stdin), &sent); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(sent["limit"], limit) {
			t.Errorf("run %d reported the limit %v, want %v", index, sent["limit"], limit)
		}
		event, ok := telemetry.DecodePlugin([]byte(call.Init.Stdin))
		if !ok || event.Limit == nil || event.Limit.Env == nil || *event.Limit.Env != 300000 || event.Limit.Settings == nil || *event.Limit.Settings != 400000 {
			t.Errorf("run %d decodes to %+v", index, event.Limit)
		}
	}
}

// A read that throws sends no limit at all, whichever of the two failed: a
// failed read is not a source that went away, and reporting it as none would
// put the model's window back in the listing until the next measure.
func TestAFailedReadSendsNoLimit(t *testing.T) {
	dir := t.TempDir()
	if err := writePlugin(filepath.Join(dir, "plugin"), []string{"/bin/rewake", "observe", "/run/s.obs"}, ""); err != nil {
		t.Fatal(err)
	}
	module, err := os.ReadFile(filepath.Join(dir, "plugin", "hooks", "rewake.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"env", "settings"} {
		world := hostWorld{
			Env:      map[string]string{"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "300000"},
			Settings: map[string]any{"autoCompactWindow": 400000},
			Fail:     []string{part},
		}
		calls, refused := runModuleIn(t, module, world, [][2]any{
			{"session.start", map[string]any{"cwd": "/w"}},
			{"session.measure", map[string]any{"context": map[string]any{"tokens": 150000, "window": 1000000, "percent": 15}, "changed": []any{"context"}}},
		})
		if len(refused) != 0 || len(calls) != 2 {
			t.Fatalf("%s failing: ran %d, refused %v", part, len(calls), refused)
		}
		for index, call := range calls {
			var sent map[string]any
			if err := json.Unmarshal([]byte(call.Init.Stdin), &sent); err != nil {
				t.Fatal(err)
			}
			if limit, ok := sent["limit"]; ok {
				t.Errorf("%s failing: run %d sent the limit %v", part, index, limit)
			}
			if event, ok := telemetry.DecodePlugin([]byte(call.Init.Stdin)); !ok || event.Limit != nil {
				t.Errorf("%s failing: run %d decodes to the limit %+v", part, index, event.Limit)
			}
		}
	}
}

// The harness refuses to load a module that uses $ inside a logical
// expression — `$.env.get(x) || …` — and the module then hears nothing at all
// (seen live on 2.1.280, docs/research-claude-control.md). The unit host
// cannot tell; this reads the module for it.
func TestTheModuleNeverUsesDollarInALogicalExpression(t *testing.T) {
	logical := regexp.MustCompile(`\|\||&&|\?\?`)
	for number, line := range strings.Split(pluginModule, "\n") {
		code, _, _ := strings.Cut(line, "//")
		if strings.Contains(code, "$.") && logical.MatchString(code) {
			t.Errorf("rewake.js:%d uses $ beside a logical operator: %s", number+1, strings.TrimSpace(line))
		}
	}
}

// The --autocompact a session is launched with reaches its collector, which
// is the only place the flag can be read: the plugin cannot see it.
func TestTheLaunchHandsTheFlagToTheCollector(t *testing.T) {
	newWorld(t)
	observation := filepath.Join(t.TempDir(), "api.obs")
	plan, err := New().Launch(harness.LaunchRequest{
		Name: "api", Dir: t.TempDir(), Socket: filepath.Join(t.TempDir(), "api.sock"), ObservationSocket: observation,
		Args: []string{"--autocompact", "1m", "--autocompact=500k"},
	})
	if err != nil {
		t.Fatal(err)
	}
	collector, ok := plan.Observer.(*telemetry.Collector)
	if !ok {
		t.Fatalf("the observer is %T", plan.Observer)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := collector.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer collector.Close()
	used, window, percent := int64(250000), int64(1000000), 25
	telemetry.Send(observation, telemetry.Event{At: 1, Kind: telemetry.PluginReady, Limit: &telemetry.Limit{}})
	telemetry.Send(observation, telemetry.Event{At: 2, Kind: telemetry.StatusLine, Context: &telemetry.Context{Used: &used, Window: &window, Percent: &percent}})
	deadline := time.Now().Add(5 * time.Second)
	for {
		snapshot := collector.SessionState()
		if snapshot.ContextWindow != nil && *snapshot.ContextWindow == 500000 && snapshot.FilledPercent != nil && *snapshot.FilledPercent == 50 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("window %v, percent %v; want the last flag's 500000 and 50", snapshot.ContextWindow, snapshot.FilledPercent)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
