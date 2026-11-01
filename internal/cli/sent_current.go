package cli

import "fmt"

// redirectLine says that a call acted on the replacement of the letter it
// named; doing is the verb, "withdrawing" or "editing".
func redirectLine(named, current, doing string) string {
	return fmt.Sprintf("Rewake: %s was replaced by %s; %s %s.", named, current, doing, current)
}
