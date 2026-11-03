// Package sessiontools holds the session tools rewake offers through every
// harness: the brief tools (docs/briefs.md) and the worktree tools
// (docs/worktree-tools.md), each defined once here — its name, description,
// input schema, the check of its input into a rewake command's arguments or a
// refusal, its bound and the text of its result. Each tool is a thin front end
// of one `rewake …` command: the operation and the right to it are the
// command's, so a tool and a command the model types answer alike.
//
// The table goes to an agent as data and its wording stays here, so a changed
// description or rule reaches a running session with the wrapper's restart
// (docs/design.md). An adapter keeps only its bridge: how the harness
// registers what it is sent, runs the command natively and shapes the result.
package sessiontools

import (
	"bytes"
	"encoding/json"
	"time"
)

// A Tool is a tool answered by one rewake command, run by argv through the
// rewake that started the run.
type Tool struct {
	// Name is the tool's name as registered; a harness may prefix it.
	Name string `json:"name"`
	// Label is a short title, for a harness that shows one.
	Label       string `json:"label"`
	Description string `json:"description"`
	// InputSchema is kept as written, so the model reads its properties in
	// the order they are described.
	InputSchema json.RawMessage `json:"inputSchema"`
	// what names the command in an error result.
	what string
	// timeout bounds the command; past it the call is reported as failed.
	timeout time.Duration
	// args is the command's arguments after the executable, or why the input
	// cannot make them: nothing runs for a refusal.
	args func(input map[string]any) ([]string, string)
}

// schema compacts a schema written for reading.
func schema(text string) json.RawMessage {
	var out bytes.Buffer
	if err := json.Compact(&out, []byte(text)); err != nil {
		panic("sessiontools: a schema is no JSON: " + err.Error())
	}
	return out.Bytes()
}

const noInput = `{"type":"object","properties":{},"additionalProperties":false}`

// statusTimeout bounds a view over records and git, seconds at most.
const statusTimeout = time.Minute

// landTimeout outlasts land's own wait of a minute for a hook: a shorter
// bound would kill a land that is only waiting.
const landTimeout = 2 * time.Minute

// briefTimeout bounds a command over a brief's records.
const briefTimeout = time.Minute

// enterTimeout bounds git worktree add and the copies .worktreeinclude asks
// for.
const enterTimeout = 2 * time.Minute

// The worktree tools: the model asks which checkout its session works in
// (worktree_status), and main lists its room's checkouts and lands one
// (worktree_list, worktree_land). Nothing from the model's input reaches the
// command but a checkout's name and a branch's, each one argument that is no
// flag, and the session is known by its run's variables, never by its
// directory.

var worktreeStatus = Tool{
	Name:        "worktree_status",
	Label:       "Worktree status",
	Description: "Say which git worktree rewake made for this session, if any — its path, branch and the commit it was made at — and what this worker's space close would do with it if it closed now: remove it (nothing would be lost; files git ignores go too), keep it with the reasons, or none (main, or no worker of a space). A look, not a promise. Takes no input.",
	InputSchema: schema(noInput),
	what:        "rewake worktree status",
	timeout:     statusTimeout,
	args:        fixed("worktree", "status", "--json"),
}

var worktreeList = Tool{
	Name:        "worktree_list",
	Label:       "Worktree list",
	Description: "List the git worktrees rewake made for the sessions of this room, each with its path, branch, the commit it was made at, the session it was made for and whether one still runs there, and what a removal would lose (changes, ignored files, commits that no branch, tag or remote-tracking ref holds, the worktree's own branch included). A listing, not a close forecast: worktree_status says what this worker's close would do. A look, not a promise. Takes no input.",
	InputSchema: schema(noInput),
	what:        "rewake worktree ls",
	timeout:     statusTimeout,
	args:        fixed("worktree", "ls", "--this-room", "--json"),
}

var worktreeLand = Tool{
	Name:        "worktree_land",
	Label:       "Worktree land",
	Description: "Fast-forward a branch to the work of one of this room's worktrees, as rewake worktree land does, and nothing else: the worktree and its session stay, hashes are kept, and it can be repeated. name is the worktree's name from worktree_list, or <repository>/<name>; into is the branch to move, by default the one checked out where the worktree was made from. Refused, with the reason and what to do: a target that moved on (rebase the branch in the worktree, then land again), one checked out elsewhere, changes in the target's checkout. Landing into the integration branch is a decision of the owner or a review: ask first.",
	InputSchema: schema(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "The worktree to land: its name, or <repository>/<name>."},
			"into": {"type": "string", "description": "The branch to fast-forward; leave out for the default."}
		},
		"required": ["name"],
		"additionalProperties": false
	}`),
	what:    "rewake worktree land",
	timeout: landTimeout,
	args:    landArgs,
}

// worktreeEnter is offered to a worker whose template lets it enter a
// checkout of its own (docs/worktree-tools.md), in a harness that can move a
// session into one; its adapter does the move.
var worktreeEnter = Tool{
	Name:        "worktree_enter",
	Label:       "Worktree enter",
	Description: "Move this session into a git worktree of its own, which rewake makes from the commit the session is at, on a new branch, and removes at this worker's close when nothing in it would be lost (landed or unchanged); otherwise the close keeps it and says why. This session starts in main's own checkout, which must not be changed: call this before your first change to any file of the repository, and before any commit or branch change, whether or not main's task mentions a worktree. One worktree per worker: called again it answers the same one, and there is no way back to the launch directory. name is optional, the worktree's and its branch's name; leave it out for a generated one.",
	InputSchema: schema(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "The worktree's name, also its branch's; leave out for a generated one."}
		},
		"additionalProperties": false
	}`),
	what:    "rewake worktree enter",
	timeout: enterTimeout,
	args:    enterArgs,
}

// The brief tools: a run main gave a brief reads it, ticks its items and
// comments.

var briefRead = Tool{
	Name:        "brief_read",
	Label:       "Brief read",
	Description: "Read a brief main gave this run with a task: a header naming its edition, the items done and the comments, then its markdown text with each item's number after its box (- [ ] #3 ...), then the comments. id is the brief's number, as the letter names it. Only a run the brief was sent to may read it, while it is open.",
	InputSchema: schema(`{
		"type": "object",
		"properties": {"id": {"type": "integer", "minimum": 1, "description": "The brief's number."}},
		"required": ["id"],
		"additionalProperties": false
	}`),
	what:    "rewake brief show",
	timeout: briefTimeout,
	args:    readArgs,
}

var briefTick = Tool{
	Name:        "brief_tick",
	Label:       "Brief tick",
	Description: "Mark an item of a brief done, or not done with undo: item is its number as brief_read shows it after the box, edition the edition brief_read's header names. Refused once main has edited the brief since that edition: read it again and tick by the new numbers. Your own ticks leave the edition as it is. Only a run the brief was sent to as work may tick; a run given it to review is refused, and says in a comment what it finds done.",
	InputSchema: schema(`{
		"type": "object",
		"properties": {
			"id": {"type": "integer", "minimum": 1, "description": "The brief's number."},
			"item": {"type": "integer", "minimum": 1, "description": "The item's number, #n in brief_read."},
			"edition": {"type": "integer", "minimum": 1, "description": "The edition the number was read in."},
			"undo": {"type": "boolean", "description": "Mark the item not done."}
		},
		"required": ["id", "item", "edition"],
		"additionalProperties": false
	}`),
	what:    "rewake brief tick",
	timeout: briefTimeout,
	args:    tickArgs,
}

var briefComment = Tool{
	Name:        "brief_comment",
	Label:       "Brief comment",
	Description: "Add a comment to a brief, at most 1000 characters, for main and the person to read under its text, shown with your name and whether you work on the brief or review it: what was found, what blocks, what was decided. Write markdown: a one-line point first, then short bullets, one idea per line, no wall of text; its lines are kept as written. It does not replace the report: that is still your final reply. Only a run the brief was sent to may comment.",
	InputSchema: schema(`{
		"type": "object",
		"properties": {
			"id": {"type": "integer", "minimum": 1, "description": "The brief's number."},
			"text": {"type": "string", "description": "The comment, at most 1000 characters."}
		},
		"required": ["id", "text"],
		"additionalProperties": false
	}`),
	what:    "rewake brief comment",
	timeout: briefTimeout,
	args:    commentArgs,
}

var all = []Tool{worktreeStatus, worktreeList, worktreeLand, worktreeEnter, briefRead, briefTick, briefComment}

// Offered is the tools a run of role is shown: main's worktree tools, or
// another run's status and brief tools, and worktree_enter for a run whose
// template lets it enter a checkout. It chooses what a session is shown, not
// what it may do: the commands behind the tools are open to any session.
func Offered(role string, enter bool) []Tool {
	var offered []Tool
	if role == "main" {
		offered = []Tool{worktreeStatus, worktreeList, worktreeLand}
	} else {
		offered = []Tool{worktreeStatus, briefRead, briefTick, briefComment}
	}
	if enter {
		offered = append(offered, worktreeEnter)
	}
	return offered
}

// Named is the tool of that name, offered to a run of role or not.
func Named(name string) (Tool, bool) {
	for _, tool := range all {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}
