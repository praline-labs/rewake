package cli

import (
	"strings"

	"github.com/praline-labs/rewake/internal/tool"
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
func ToolDescriptors() []tool.Descriptor { return surfaceTable().toolDescriptors() }

func (t *table) toolDescriptors() []tool.Descriptor {
	tools := make([]tool.Descriptor, 0, len(toolOrder))
	for _, name := range toolOrder {
		command := t.findCommand(name)
		if command == nil {
			continue
		}
		descriptor := tool.Descriptor{Name: name, Summary: command.Summary}
		for _, flag := range toolFlags[name] {
			option, ok := t.lookupOption(command, flag)
			if !ok {
				continue
			}
			kind := tool.ParamSwitch
			if option.Value != "" {
				kind = tool.ParamValue
			}
			descriptor.Params = append(descriptor.Params, tool.Param{Name: flag, Kind: kind, Description: option.Summary})
		}
		for index, placeholder := range positionalNames(command) {
			description := ""
			if index < len(toolPositionals[name]) {
				description = toolPositionals[name][index]
			}
			descriptor.Params = append(descriptor.Params, tool.Param{Name: placeholder, Kind: tool.ParamWord, Description: description, Required: true})
		}
		tools = append(tools, descriptor)
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
