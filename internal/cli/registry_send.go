package cli

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/inbox"
)

// sendCommand is send's entry in the command table.
func sendCommand() *Command {
	return &Command{
		Name:           "send",
		Args:           "<name> <text>",
		MaxPositionals: 2,
		Summary:        "Give a session a task, a question or a heads-up. Use - as the text to read it from stdin.",
		Options: append(kindOptions(),
			Option{Flag: "--grant-git", Summary: "Verified main only: let a task or question's recipient write its repository's validated Git metadata. Only a write session whose harness takes a Git grant receives it; Claude Code takes none, and is refused with exit 1."},
			Option{Flag: "--grant-dir", Value: "<dir>", Repeatable: true, Summary: "Verified main only: let a task or question's recipient write this directory while the task is open. Repeatable, at most 8 together with --grant-dir-broad; a relative path is taken from your directory. The notes say when it ends, what is never granted and how long the task waits."},
			Option{Flag: "--grant-dir-broad", Value: "<dir>", Repeatable: true, Summary: "The same for a directory holding many others, named exactly, which --grant-dir refuses: a drive such as /mnt/d, a directory directly in your home, a ~/.config/<X> keeping credentials, one where a live rewake session works or that holds one, one with .git, .claude, .codex or .agents in its path."},
			Option{Flag: "--wait", Value: "<seconds>", Summary: "How long to wait for the delivery, default 5, or for a question's answer, default 600, of which the delivery takes 5 at most."},
			Option{Flag: "--to", Value: "<id>", Summary: "Add to an unreported task or question this run sent: a task of its own, announced at once, shown with it by rewake inbox --owed and settled by the same report. Takes a unique prefix of the id; an id rewake edit replaced adds to the replacement."},
			jsonOption,
		),
		Examples: append(sendExamples(),
			"rewake send writer --grant-git \"Commit the reviewed change\"",
			"rewake send writer --grant-dir ../shared-lib \"Bump the client in shared-lib as well\"",
			"rewake send writer --grant-dir-broad /mnt/d \"Sort the exports on the D: drive\"",
		),
		Next: []string{"rewake inbox"},
		Notes: []string{
			"A task is the default: the session works and ends its turn, and its final message comes back to you as a \"Rewake: <session> finished\" line.",
			"A question blocks until that final message and prints it; run a long one in the background.",
			fmt.Sprintf("A notify waits up to %s to share one notice with other mail, and send waits with it; a task or a question is announced at once.", inbox.Coalescing.Cap),
			"A verified main sees the answering session's state line above a question's answer; rewake list --help says what it holds.",
			"Quote the text as one argument: loose words are refused, not joined.",
			"A granted directory is resolved to its real path, checked again at delivery and handed over only while the session is idle, so it holds from the task's first turn. It lasts while the task is open, through a stopped or pending turn. The report, an error that ends the turn, or rewake withdraw settles the task, and the grant is taken back after that, when depending on the harness. A grant is for writing; what a session may read is its harness's business.",
			"--grant-git alone opens the Git metadata of the session's own checkout for the rest of the thread: rewake does not journal it, so neither the report nor a later message takes it back, only a typed turn that replaces the roots. Grant it only to a session you would let commit from then on.",
			"A task carrying a grant waits at most 30 minutes for the session to be idle, then expires and you are told. Your wrapper confirms the grant at delivery, so a task whose main has ended is not delivered. A main whose harness runs its commands where they cannot reach its wrapper cannot grant.",
			"On Claude Code the grant goes through the permission hook: a file-tool write inside the directory is allowed without asking, and a shell command there too once such a write added the directory. In the default and acceptEdits modes it is taken back at the first read or rewake command after the report; in plan, bypassPermissions and auto it stays until the session ends. It spares prompts and is no boundary: an approved shell command writes anywhere anyway. A .git, .claude, .codex or .agents at any depth inside is left to the person there.",
			"After a cold resume of the recipient, its grants come back only while your session runs, into the same conversation, within a day of the task's read: the resumed run asks your wrapper. Claude Code gets them with --resume <id> or at the first write after --continue, not with /resume. A main that restarted or ended restores nothing.",
			"Until a task carrying a grant is delivered, --to refuses to add to it, since the addition would be read first; change it with rewake edit <id>.",
			"Never granted, with exit 2: rewake's state, configuration and binary; a harness's configuration; ~/.ssh, ~/.gnupg, ~/.aws, ~/.kube, ~/.docker, ~/.password-store, ~/.config/gh; ~/.config/git, systemd, autostart, environment.d and fish; ~/.local/share/systemd and applications; the Go module and build caches; a directory on PATH and the toolchain above it; /tmp and $TMPDIR, where a worker could swap the directory for a link, and any path through one; a Windows short name rewake cannot read back to its long one; / and the system directories — nor anything inside or above these. The owner can give one at launch or in the session's terminal.",
			"A directory already in the recipient's workspace is named as already writable and not granted; one inside another granted directory goes with it.",
			"The output ends with the message's id, which withdraw, edit and --to take whole or as a unique prefix of it or of its part after the dash.",
		},
		Handler: handleSend,
	}
}
