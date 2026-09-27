package codex

import "strings"

// legacyLandlockKey is the Codex feature that runs sandboxed commands under
// Landlock alone, in the app-server's own namespaces. A grant is confirmed
// between two wrappers, and the confirmation trusts that no worker process
// shares their mount, user and PID namespaces; under this feature one does,
// and can answer in main's name from a process it started itself
// (docs/grants.md#who-can-grant). Probed on codex-cli 0.155.1 and 0.157.1,
// 2026-09-26: off by default, and only an explicit setting turns it on.
const legacyLandlockKey = "use_legacy_landlock"

// legacyLandlock says why a session may run its commands in rewake's own
// namespaces, and nothing when it cannot. Like configMentions, a mention in
// any form is enough: reading the value wrong would hand a grant to a worker
// that can forge one, while a false alarm only refuses grants to one session.
func legacyLandlock(args []string, home string) string {
	for _, arg := range args {
		if strings.Contains(arg, legacyLandlockKey) || strings.Contains(arg, "legacy-landlock") {
			return "its launch arguments mention " + legacyLandlockKey
		}
	}
	if mentioned, why := configMentions(home, legacyLandlockKey); mentioned {
		return why
	}
	return ""
}

// legacyLandlockRefusal is the refusal of a grant to such a session.
func legacyLandlockRefusal(why string) string {
	return "this session cannot take a grant: " + why + ", and under legacy Landlock its commands run beside rewake, where a worker could confirm a grant in main's name; start it without that feature"
}
