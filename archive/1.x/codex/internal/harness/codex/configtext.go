package codex

import (
	"os"
	"path/filepath"
	"strings"
)

/*
rewake does not read the Codex configuration. It only asks whether a key is
mentioned in it at all.

Every value rewake could pass for these keys replaces the user's rather than
adding to it, and two rounds of a hand-written reader each missed valid TOML
that turned a user's instructions into nothing, or their prose into sandbox
permissions. Reading too little costs a briefing or a note; reading wrong costs
the user's configuration. So a mention, in any form, is enough to leave the key
alone and say so.
*/

// sandboxSection holds the write permissions of the Codex sandbox.
const sandboxSection = "sandbox_workspace_write"

// configPath is the user's Codex configuration.
func configPath(home string) string { return filepath.Join(home, "config.toml") }

// configMentions reports whether the configuration may set a key, and why.
// It answers yes when the file cannot be read, and when it holds escapes: a
// quoted key can spell any name with them.
func configMentions(home, key string) (bool, string) {
	raw, err := os.ReadFile(configPath(home))
	if os.IsNotExist(err) {
		return false, ""
	}
	if err != nil {
		return true, "config.toml could not be read: " + err.Error()
	}
	text := string(raw)
	if strings.Contains(text, key) {
		return true, "config.toml mentions " + key
	}
	if strings.Contains(text, `\u`) || strings.Contains(text, `\U`) {
		return true, "config.toml has escaped characters rewake does not decode, and one of them could spell " + key
	}
	return false, ""
}
