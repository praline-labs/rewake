package cli

import "github.com/iiiokojiadbi/rewake/internal/harness"

// Match the guide's general-role launch instead of implying that --name is
// already a complete address. Direct sends still accept existing names as-is.
func sendExamples() []string {
	address := "helper"
	if available := harness.All(); len(available) > 0 {
		address += "-" + available[0].ID()
	}
	return []string{
		"rewake send " + address + " \"rerun the smoke and report what failed\"",
		"rewake send " + address + " \"which port does the dev server use?\" --question",
		"rewake send " + address + " \"the migration is merged\" --notify",
		"rewake send " + address + " - --wait 20",
	}
}
