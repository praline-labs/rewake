package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/screen"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/worktree"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// screenView is what rewake screen answers.
type screenView struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
	// Cursor is where the cursor is, from 0, when the pane shows one.
	Cursor *screen.Cursor `json:"cursor,omitempty"`
	// StandIn names the colors the terminal did not report, which the picture
	// draws in stand-ins of its own.
	StandIn []string `json:"standIn,omitempty"`
	// PNG is the file written, when --png asked for one.
	PNG string `json:"png,omitempty"`
	// Text is the screen's lines, less their trailing blanks; left out when
	// a picture was asked for and --text was not.
	Text []string `json:"text,omitempty"`
}

func (v screenView) lines() []string {
	what := "window"
	if v.Source == screen.SourceHost {
		what = "pane alone, from its space host (no shell is running)"
	}
	out := []string{fmt.Sprintf("Rewake: %s's %s, %dx%d", v.Name, what, v.Cols, v.Rows)}
	if v.PNG != "" {
		out = append(out, "png: "+v.PNG)
	}
	if len(v.StandIn) > 0 {
		out = append(out, "stand-in colors (the terminal did not report them): "+strings.Join(v.StandIn, ", "))
	}
	return append(out, v.Text...)
}

func screenCommand() *Command {
	return &Command{
		Name: "screen", Args: "<name>", MaxPositionals: 1,
		Summary: "A picture of a session's window as the person sees it in rewake's shell: its pane, its status line and the navigation bar. Text on stdout, a PNG with --png.",
		Options: []Option{
			{Flag: "--png", Value: "<path>", Summary: "Write the picture there, a PNG; the path must lie outside rewake's state."},
			{Flag: "--text", Summary: "Print the screen's text as well as writing the PNG; without --png the text is printed anyway."},
			jsonOption,
		},
		Examples: []string{"rewake screen helper-claude", "rewake screen helper-claude --png /tmp/helper.png", "rewake screen main --png /tmp/main.png --text --json"},
		Notes: []string{
			"<name> is a worker, by its space name or its address from rewake list, or main (main, or main's own address). A view: any session of the room may take it, it writes nothing of the room, and the person's screen is not touched.",
			"The shell draws the window off-screen at the size the person's window has now, in the layout it has now, with the named session in its place: the worker's pane with its status line and the bar under it, main's half beside it in a wide window. Main's window is main's, even while the shell shows a worker. Colors and attributes are in the picture and not in the text.",
			"With no shell running (main started without a terminal), a worker's pane alone comes from its space host's emulator and is marked so: source host. Main has no host, and the call is refused with exit 1.",
			"The picture is drawn with a font that ships with rewake, 8 by 16 pixels a cell, wide glyphs and box drawing included; at most 400 by 200 cells. --png refuses a path inside rewake's state, through a symbolic link, or onto an existing file that is not a regular one; an existing regular file is replaced.",
			"Exit 0 done; 1 the session could not be drawn (no shell and no host answering, the shell refused it, the picture could not be written); 2 a wrong call — no such session, a path refused.",
		},
		Access: viewForm, Handler: handleScreen,
	}
}

func handleScreen(ctx *Context, call Call) error {
	name := call.Positionals[0]
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	pane, hostSocket, err := screenTarget(dir, name)
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	png := call.Flag("png", "")
	if _, given := call.Flags["png"]; given && png == "" {
		return &UsageError{Command: call.Command, Message: "--png takes the path to write: rewake screen " + name + " --png /tmp/screen.png."}
	}
	frame, err := askShell(state.SocketPath(dir, "shell", ""), pane)
	if errors.Is(err, errNoShell) {
		if hostSocket == "" {
			return failf("main's window is drawn by the shell, and no shell is running in this room: main has no space host to show its pane from")
		}
		frame, err = askHost(hostSocket)
	}
	if err != nil {
		return failf("could not take %s's screen: %v", name, err)
	}
	if err := frame.Check(); err != nil {
		return failf("could not take %s's screen: %v", name, err)
	}
	view := screenView{Name: name, Source: frame.Source, Cols: frame.Cols, Rows: frame.Rows, Cursor: frame.Cursor, StandIn: frame.StandIn()}
	if png == "" || call.Switch("text") {
		view.Text = frame.Text()
	}
	if png != "" {
		data, err := frame.PNG()
		if err != nil {
			return failf("could not draw %s's screen: %v", name, err)
		}
		keep, err := screenKeep()
		if err != nil {
			return failf("could not tell rewake's state from %s: %v", png, err)
		}
		if view.PNG, err = screen.WritePNG(png, data, keep); err != nil {
			return &UsageError{Command: call.Command, Message: err.Error()}
		}
	}
	return printValue(ctx, view, view.lines)
}

// screenTarget is the pane the shell is asked for — main, or a space's id —
// and the socket of the worker's space host, empty for main. A name is a
// space's, a worker's address, or a main's.
func screenTarget(dir, name string) (pane, hostSocket string, err error) {
	records, _, err := space.List(dir)
	if err != nil {
		return "", "", fmt.Errorf("could not read the spaces: %v", err)
	}
	for _, record := range records {
		if record.State.Final() {
			continue
		}
		if name == record.Name || name == record.Worker.Name+"-"+record.Worker.Harness {
			return record.ID, state.SpaceSocketPath(dir, record.ID), nil
		}
	}
	if name == "main" {
		return "main", "", nil
	}
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return "", "", fmt.Errorf("could not read the sessions: %v", err)
	}
	for _, session := range sessions {
		if session.Name == name && session.Role == role.Main.ID {
			return "main", "", nil
		}
	}
	return "", "", fmt.Errorf("no worker or main named %q in this room: rewake space list shows the workers, rewake list the sessions", name)
}

// screenKeep are the folders a picture is never written in: the state
// directory with every room in it, and rewake's folder of the person's
// configuration.
func screenKeep() ([]string, error) {
	base, err := state.Base()
	if err != nil {
		return nil, err
	}
	// The default base is kept even when REWAKE_DIR names another: a room
	// there is still rewake's state.
	keep := []string{base, filepath.Join(os.TempDir(), "rewake-"+strconv.Itoa(os.Getuid()))}
	if root, err := worktree.Root(); err == nil {
		keep = append(keep, root)
	}
	if config, err := os.UserConfigDir(); err == nil {
		keep = append(keep, filepath.Join(config, "rewake"))
	}
	return keep, nil
}
