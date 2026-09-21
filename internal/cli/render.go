package cli

import (
	"fmt"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/role"
)

// wrapWidth keeps prose readable in a narrow terminal without reflowing to it:
// the output must be identical everywhere, including in a pipe.
const wrapWidth = 96

// sprintf is fmt.Sprintf under a shorter name, used by refusal builders.
func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// column is one row of a two-column block.
type column struct {
	Name string
	Text string
}

// printColumns pads names into a column so a scanning eye — or a regexp — finds
// the text at a stable offset.
func printColumns(rows []column, indent string) []string {
	if len(rows) == 0 {
		return nil
	}
	width := 0
	for _, row := range rows {
		if len(row.Name) > width {
			width = len(row.Name)
		}
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		padded := row.Name + strings.Repeat(" ", width-len(row.Name))
		line := indent + padded
		if row.Text != "" {
			line += "  " + row.Text
		}
		out = append(out, strings.TrimRight(line, " "))
	}
	return out
}

// wrapText breaks prose at wrapWidth, indenting every line.
func wrapText(text, indent string) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := ""
	for _, word := range words {
		switch {
		case current == "":
			current = word
		case len(current)+1+len(word)+len(indent) <= wrapWidth:
			current += " " + word
		default:
			lines = append(lines, indent+current)
			current = word
		}
	}
	return append(lines, indent+current)
}

// formatGuide renders the overview printed when rewake is called with nothing.
// An agent's first call in a session should teach how the tool behaves, which a
// refusal cannot do.
func formatGuide(play *role.Playbook) string {
	var lines []string
	lines = append(lines, "rewake — let coding agents on this machine message each other.", "")

	// The caller's own role first, when the caller is a session. What an agent
	// needs is the handful of moves its role makes, and it needs them before
	// the map of everything the tool can do.
	if play != nil {
		lines = append(lines, "YOUR ROLE", "  "+play.Heading, "")
		rows := make([]column, 0, len(play.Steps))
		for _, step := range play.Steps {
			rows = append(rows, column{Name: step.Do, Text: step.Why})
		}
		lines = append(lines, printColumns(rows, "  ")...)
		lines = append(lines, "")
		for _, limit := range play.Limits {
			lines = append(lines, wrapText(limit, "  ")...)
		}
		lines = append(lines, "")
	}

	for _, group := range Groups() {
		shown := visibleCommands(group)
		if len(shown) == 0 {
			continue
		}
		lines = append(lines, group.Title)
		if group.Summary != "" {
			lines = append(lines, wrapText(group.Summary, "  ")...)
			lines = append(lines, "")
		}
		rows := make([]column, 0, len(shown))
		for _, command := range shown {
			rows = append(rows, column{Name: command.Label(), Text: command.Summary})
		}
		lines = append(lines, printColumns(rows, "  ")...)
		lines = append(lines, "")
	}

	lines = append(lines, "FLOW")
	steps := make([]column, 0)
	for _, step := range flow() {
		steps = append(steps, column{Name: step.Command, Text: step.Summary})
	}
	lines = append(lines, printColumns(steps, "  ")...)
	lines = append(lines, "")

	lines = append(lines, formatGlobalOptions()...)
	lines = append(lines, "")

	lines = append(lines, "HOW THIS TOOL BEHAVES")
	for _, note := range notes() {
		lines = append(lines, "  "+note.Title)
		lines = append(lines, wrapText(note.Body, "    ")...)
		lines = append(lines, "")
	}

	lines = append(lines, "Flags and examples for one command: rewake <command> --help")
	return strings.Join(lines, "\n")
}

// formatGlobalOptions renders the global flag block.
func formatGlobalOptions() []string {
	rows := make([]column, 0, len(globalOptions))
	for _, option := range globalOptions {
		rows = append(rows, column{Name: option.Label(), Text: option.Summary})
	}
	return append([]string{"GLOBAL OPTIONS"}, printColumns(rows, "  ")...)
}

// formatCommandHelp renders the help page of one command.
func formatCommandHelp(command *Command, includeGlobals bool) string {
	lines := []string{"rewake " + command.Label(), ""}
	lines = append(lines, wrapText(command.Summary, "  ")...)

	local := make([]column, 0, len(command.Options))
	for _, option := range command.Options {
		if option.Flag == jsonOption.Flag {
			continue
		}
		text := option.Summary
		if option.Required {
			text = "Required. " + text
		}
		local = append(local, column{Name: option.Label(), Text: text})
	}
	if len(local) > 0 {
		lines = append(lines, "", "OPTIONS")
		lines = append(lines, printColumns(local, "  ")...)
	}

	if len(command.Notes) > 0 {
		lines = append(lines, "", "USAGE NOTES")
		for _, note := range command.Notes {
			lines = append(lines, wrapText(note, "  ")...)
		}
	}

	if len(command.Examples) > 0 {
		lines = append(lines, "", "EXAMPLES")
		for _, example := range command.Examples {
			lines = append(lines, "  "+example)
		}
	}

	if len(command.Next) > 0 {
		lines = append(lines, "", "USUALLY NEXT")
		for _, next := range command.Next {
			lines = append(lines, "  "+next)
		}
	}

	if includeGlobals {
		lines = append(lines, "")
		lines = append(lines, formatGlobalOptions()...)
	}
	return strings.Join(lines, "\n")
}

// formatHint is appended to a refusal that names a command: syntax, examples and
// flags, so the next call can be right without a second round trip.
func formatHint(command *Command) string {
	if command == nil {
		return ""
	}
	lines := []string{"", "  rewake " + command.Label(), ""}
	for _, example := range command.Examples {
		lines = append(lines, "  "+example)
	}
	local := make([]column, 0, len(command.Options))
	for _, option := range command.Options {
		local = append(local, column{Name: option.Label(), Text: option.Summary})
	}
	if len(local) > 0 {
		lines = append(lines, "", "  flags:")
		lines = append(lines, printColumns(local, "    ")...)
	}
	lines = append(lines, "", "  full help: rewake "+command.Name+" --help")
	return strings.Join(lines, "\n")
}
