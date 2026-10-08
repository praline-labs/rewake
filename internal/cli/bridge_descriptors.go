package cli

import (
	"strings"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The tools a harness offers its model, built from the command table and the
// allowlist (docs/v2/design-api.md#tooltransport): one per allowed command,
// its flags as the table states them and only those the allowlist lets
// through. The call comes back as words for a child of the wrapper's own
// image, which parses them with the same table and checks them against the
// same allowlist again (ToolWords), so a descriptor cannot widen the surface.

// toolOrder is the order the tools are offered in.
var toolOrder = []string{"inbox", "send", "pending", "whoami", "retry", "list"}

// toolPositionals names, in the table's order, what each positional of an
// allowed command is; the table's Args holds only placeholders.
var toolPositionals = map[string][]string{
	"pending": {"what the turn ends before: the work it waits for"},
	"send":    {"the session the heads-up goes to", "the text it is told"},
	"retry":   {"the receipt of the call to continue or reconcile, as its answer printed it"},
}

// ToolDescriptors are the tools of the surface, in the order they are offered.
func ToolDescriptors() []bridge.ToolDescriptor {
	tools := make([]bridge.ToolDescriptor, 0, len(toolOrder))
	for _, name := range toolOrder {
		command := findCommand(name)
		if command == nil {
			continue
		}
		tool := bridge.ToolDescriptor{Name: name, Summary: command.Summary}
		for _, flag := range toolFlags[name] {
			option, ok := lookupOption(command, flag)
			if !ok {
				continue
			}
			kind := bridge.ParamSwitch
			if option.Value != "" {
				kind = bridge.ParamValue
			}
			tool.Params = append(tool.Params, bridge.ToolParam{Name: flag, Kind: kind, Description: option.Summary})
		}
		for index, placeholder := range positionalNames(command) {
			description := ""
			if index < len(toolPositionals[name]) {
				description = toolPositionals[name][index]
			}
			tool.Params = append(tool.Params, bridge.ToolParam{Name: placeholder, Kind: bridge.ParamWord, Description: description, Required: true})
		}
		tools = append(tools, tool)
	}
	return tools
}

// positionalNames are the placeholders of a command's Args, without their
// brackets: "<name> <text>" names name and text.
func positionalNames(command *Command) []string {
	var names []string
	for _, field := range strings.Fields(command.Args) {
		names = append(names, strings.Trim(field, "<>"))
	}
	return names
}
