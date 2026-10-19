//go:build rewaketest

package cli

import (
	"strings"

	"github.com/praline-labs/rewake/internal/core/role"
)

// The test build keeps a direct launch of any role in any room: the workflow
// suite starts its workers that way, without a main to open their spaces
// (main's decision 3, October 19, 2026 — docs/v2/worker-profile.md).

var nameOption = Option{
	Flag:    "--name",
	Value:   "<prefix>",
	Summary: "Address prefix, the role by default: the session is <prefix>-<harness>, 32 characters at most. A taken default gets -2, -3; a taken --name is refused. Test build only.",
}

var roomOption = Option{
	Flag: "--room", Value: "<name>",
	Summary: "The room to join; the launch directory's otherwise. Test build only.",
}

func directIdentity(call Call) (launchIdentity, error) {
	if prefix, present := call.Flags["name"]; present && prefix == "" {
		return launchIdentity{}, &UsageError{Command: call.Command, Message: "--name needs a nonempty prefix; omit --name to use the selected role."}
	}
	part, err := chosenRole(call)
	if err != nil {
		return launchIdentity{}, err
	}
	id := launchIdentity{room: call.Flag("room", "")}
	if id.room == "" {
		if id, err = ownRoom(); err != nil {
			return launchIdentity{}, err
		}
	}
	id.name = call.Flag("name", "")
	if part.ID != "" {
		id.part, id.reason = part, "selected explicitly with --"+part.ID
	} else if id.part = role.Main; id.reason == "" {
		id.reason = "main by default"
	}
	return id, nil
}

func directOptions() []Option { return append([]Option{nameOption, roomOption}, roleOptions()...) }

// chosenRole is the role flag given, unset when none is.
func chosenRole(call Call) (role.Role, error) {
	chosen := role.Role{}
	var flags []string
	for _, candidate := range role.All() {
		if call.Switch(candidate.ID) {
			chosen = candidate
			flags = append(flags, "--"+candidate.ID)
		}
	}
	if len(flags) > 1 {
		return role.Role{}, &UsageError{
			Command: call.Command,
			Message: strings.Join(flags, " and ") + " exclude each other: a session has one role.",
		}
	}
	return chosen, nil
}

// roleOptions are the launch flags that choose a role, for the command table.
func roleOptions() []Option {
	var options []Option
	for _, candidate := range role.All() {
		options = append(options, Option{Flag: "--" + candidate.ID, Summary: candidate.Summary + " Test build only."})
	}
	return options
}
