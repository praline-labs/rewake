package channel

// Category is a derived state of the channel, from a closed list
// (docs/mail-bridge-channel.md#what-is-shown).
type Category string

// The categories, in the order they are decided: the block first, then the
// tool, then the shell observed since the interval opened.
const (
	CategoryDenied      Category = "denied"
	CategoryTool        Category = "tool"
	CategoryPending     Category = "starting"
	CategoryFailing     Category = "failing"
	CategoryShell       Category = "shell"
	CategoryNoChannel   Category = "none"
	CategoryNoTool      Category = "no tool"
	categoryUnknownWord          = "unknown"
)

// Category derives the category from the record.
func (r *Record) Category() Category {
	switch {
	case r.Blocked():
		return CategoryDenied
	case !r.Open() && r.Tool == ToolWorking:
		return CategoryTool
	case !r.Open():
		return CategoryPending
	}
	if shell := r.Shell; shell != nil && shell.At.Boot >= r.Interval.Boot {
		if shell.OK {
			return CategoryShell
		}
		return CategoryNoChannel
	}
	if r.Tool == ToolNone {
		return CategoryNoTool
	}
	return CategoryFailing
}

// Word is the category as rewake list shows it; a run with no record shows
// that it is unknown.
func Word(r *Record) string {
	if r == nil {
		return categoryUnknownWord
	}
	return string(r.Category())
}

// timeFormat is how a line shows a time: the wall clock, to the second.
const timeFormat = "2006-01-02 15:04:05"

func shown(at Stamp) string { return at.Wall.Local().Format(timeFormat) }

// Line is the line whoami, rewake list and main's header show, without its
// "mail: " label.
func (r *Record) Line() string {
	switch r.Category() {
	case CategoryDenied:
		return "tool denied by policy"
	case CategoryTool:
		return "tool"
	case CategoryPending:
		switch r.Tool {
		case ToolConnected:
			return "tool connected, unused"
		case ToolNotConnected:
			return "tool not connected"
		}
		return "tool starting"
	case CategoryFailing:
		line := "tool failing (" + r.Class + ") since " + shown(r.Interval) + "; shell unconfirmed"
		if r.Worked.Boot != 0 {
			line += "; tool last worked " + shown(r.Worked)
		}
		return line + r.reconnected()
	case CategoryShell:
		return "through the shell since " + shown(r.Shell.At) + "; " + r.toolPart() + r.reconnected()
	case CategoryNoChannel:
		return "no working channel: " + r.toolPart() + ", shell " + r.Shell.Class + r.reconnected()
	}
	return "no tool (" + r.Reason + "); shell unconfirmed"
}

// toolPart names the tool's side of an open interval.
func (r *Record) toolPart() string {
	if r.Tool == ToolNone {
		return "no tool (" + r.Reason + ")"
	}
	return "tool failing (" + r.Class + ")"
}

// reconnected notes a connection that came back during the interval: it
// changes neither the category nor the class, so it sends no notice.
func (r *Record) reconnected() string {
	if !r.Open() || r.Reconnected.Boot == 0 || r.Tool == ToolNone {
		return ""
	}
	return "; reconnected " + shown(r.Reconnected) + ", unused"
}

// Label is the line with its label, as every surface prints it.
func Label(r *Record) string {
	if r == nil {
		return "mail: " + categoryUnknownWord
	}
	return "mail: " + r.Line()
}
