//go:build !rewaketest

package cli

import (
	"strings"
	"testing"
)

// The flags that made a person's launch a worker, named it, or put it in
// another room are gone from the release build.
func TestALaunchTakesNoRoleNameRoomOrShell(t *testing.T) {
	for _, flags := range [][]string{{"--main"}, {"--write"}, {"--general"}, {"--name", "api"}, {"--room", "red"}, {"--shell"}} {
		if _, err := testTable().parse(append(flags, agentID)); err == nil {
			t.Errorf("%v accepted", flags)
		}
	}
	code, out, _ := run(agentID, "--help")
	for _, flag := range []string{"--main", "--write", "--general", "--name", "--room", "--shell"} {
		if code != ExitOK || strings.Contains(out, flag+" ") {
			t.Errorf("help still shows %s:\n%s", flag, out)
		}
	}
}
