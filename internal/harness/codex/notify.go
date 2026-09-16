package codex

import (
	"os"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// notifyKey names the program Codex runs after every turn, with a JSON payload
// as its last argument. It runs outside the sandbox and needs no trust, unlike
// a Stop hook, which Codex skips silently unless the hook was approved.
const notifyKey = "notify"

// turnNotify returns the notify override that reports the end of turns, or the
// reason it is not passed.
//
// The key replaces the user's value rather than adding to it, and a notify
// program is not something two callers can share. So the override goes in only
// when it is certain the user has none: the key does not appear anywhere in
// the configuration, not even as text. Reading too little costs the end-of-turn
// notices; reading wrong would silently switch off the user's own program.
func turnNotify(home string, args []string, layered bool) (string, string) {
	const skipped = "not reporting the end of turns to the sessions that wrote here: "
	switch {
	case layered:
		return "", skipped + "a profile is selected and its notify setting cannot be read from here"
	case hasConfigKey(args, notifyKey):
		return "", skipped + "notify is already given on the command line"
	}
	raw, err := os.ReadFile(configPath(home))
	if err != nil && !os.IsNotExist(err) {
		return "", skipped + "config.toml could not be read: " + err.Error()
	}
	if strings.Contains(string(raw), notifyKey) {
		return "", skipped + "config.toml mentions notify, and passing rewake's own would replace it"
	}
	// A quoted key may spell notify with escapes. Decoding TOML is not this
	// reader's job, so any escape at all counts as a possible notify.
	if strings.Contains(string(raw), `\u`) || strings.Contains(string(raw), `\U`) {
		return "", skipped + "config.toml has escaped characters rewake does not decode, and one of them could spell notify"
	}
	argv, err := harness.TurnEndedArgv()
	if err != nil {
		return "", skipped + err.Error()
	}
	return quoteTOMLArray(argv), ""
}
