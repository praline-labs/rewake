package codex

import (
	"strings"
	"testing"
)

func TestLaunchRefusesWritableRoots(t *testing.T) {
	refused := [][]string{
		{"--add-dir", "/w/a"},
		{"--add-dir=/w/a"},
		{"-c", "sandbox_workspace_write.writable_roots=[\"/w/a\"]"},
		{"--config", " sandbox_workspace_write.writable_roots = []"},
		{"-csandbox_workspace_write.writable_roots=[]"},
		{"-c", "profiles.work.sandbox_workspace_write.writable_roots=[]"},
		{"-c", "sandbox_workspace_write={writable_roots=[\"/w/a\"]}"},
	}
	for _, args := range refused {
		err := codexHarness{}.RefuseLaunch(append([]string{"--model", "m"}, args...))
		if err == nil || !strings.Contains(err.Error(), "--grant-dir") {
			t.Errorf("%q: got %v, want a refusal naming --grant-dir", args, err)
		}
	}
	allowed := [][]string{
		{"-c", "sandbox_workspace_write.network_access=true"},
		{"-c", "sandbox_workspace_write={network_access=true}"},
		{"--", "--add-dir /w/a is part of the prompt"},
		{"-c", "model=\"writable_roots\""},
	}
	for _, args := range allowed {
		if err := (codexHarness{}).RefuseLaunch(args); err != nil {
			t.Errorf("%q: refused: %v", args, err)
		}
	}
}
