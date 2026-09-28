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
			Option{Flag: "--grant-git", Summary: "Verified main only: explicitly grant eligible task/question recipients access to validated repository Git metadata. No flag adds no roots."},
			Option{Flag: "--grant-dir", Value: "<dir>", Repeatable: true, Summary: "Verified main only: let a task or question's recipient write this directory until it reports. Repeat for more, at most 8 with --grant-dir-broad. A relative path is taken from your directory. See the notes for what is never granted, and for how long the task may wait."},
			Option{Flag: "--grant-dir-broad", Value: "<dir>", Repeatable: true, Summary: "The same for a directory that holds many others — /home, /mnt, a drive, a directory directly in your home, a ~/.config/<X> keeping credentials, a directory where a live rewake session works or that holds one — named exactly; --grant-dir refuses those."},
			Option{Flag: "--wait", Value: "<seconds>", Summary: "How long to wait. Default: 5 for the delivery, 600 for a question's answer."},
			Option{Flag: "--to", Value: "<id>", Summary: "Add to a task or question this run sent that is not reported on yet: a task of its own, announced at once, shown with it by rewake inbox --owed and settled by the same report. Takes a unique prefix of the id; an id rewake edit replaced adds to its replacement."},
			jsonOption,
		),
		Examples: append(sendExamples(),
			"rewake send writer-codex --grant-git \"Commit the reviewed change\"",
			"rewake send writer-codex --grant-dir ../shared-lib \"Bump the client in shared-lib as well\"",
			"rewake send writer-codex --grant-dir-broad /mnt/d \"Sort the exports on the D: drive\"",
		),
		Next: []string{"rewake inbox"},
		Notes: []string{
			"A task is the default: the session reads it, works, and ends its turn with a final message, which comes back to you as a \"Rewake: <session> finished\" line.",
			"A question blocks until that final message and prints it. A long one is better run in the background.",
			fmt.Sprintf("A heads-up (--notify) waits up to %s to share one notice with other mail arriving meanwhile, and send waits with it; a task or a question is announced at once.", inbox.Coalescing.Cap),
			sessionStateHelp,
			"Quote the text as one argument: loose words are refused rather than silently joined.",
			"A granted directory is resolved to its real path, checked again when the task is delivered, and handed over only while the session is idle, so the grant holds from the task's first turn; it is taken back once the task is reported on. A grant is for writing: what a session may read is its harness's own business.",
			"A task carrying a grant waits at most 30 minutes, its time to live, for the session to be idle; past that it expires and you are told. Your session's wrapper confirms the grant when it is delivered, so a task whose main has ended since is not delivered. A Codex main cannot grant: its sandbox does not reach its wrapper.",
			"On a Claude Code session the grant goes through its permission hook: a file-tool write inside the directory is allowed without asking, and a shell command there too once such a write has added the directory; it is taken back at the first read or rewake command after the report, in the default and acceptEdits modes. It removes prompts and is no boundary: an approved shell command of that session writes anywhere anyway. A .git inside the directory is left to the person there.",
			"While a task carrying a grant is not delivered, --to cannot add to it, since the addition would be read first; change the task with rewake edit <id>.",
			"Never granted, with exit 2: rewake's state, configuration and binary; a harness's own configuration; ~/.ssh, ~/.gnupg, ~/.aws, ~/.kube, ~/.docker, ~/.password-store; ~/.config/git, systemd, autostart and environment.d; a directory on PATH; /tmp and $TMPDIR, where a worker could swap the directory for a link; / and the system directories — nor a directory inside or above one of them. A session that truly needs one gets it from the owner, at launch or in its own terminal.",
			"A directory already in the recipient's workspace is not granted and is named as already writable; a directory inside another granted one goes with it.",
			"The output ends with the message's id, which rewake withdraw, rewake edit and --to take, whole or as a unique prefix of it or of the part after the dash.",
		},
		Handler: handleSend,
	}
}
