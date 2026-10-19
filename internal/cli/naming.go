package cli

import "github.com/praline-labs/rewake/internal/adapter"

// Match the guide's worker opened as helper instead of implying that a space's
// name is already a complete address. Direct sends still accept existing names as-is.
func sendExamples(catalog adapter.Catalog) []string {
	address := "helper"
	if available := catalog.All(); len(available) > 0 {
		address += "-" + available[0].ID()
	}
	return []string{
		"rewake send " + address + " \"rerun the smoke and report what failed\"",
		"rewake send " + address + " \"which port does the dev server use?\" --question",
		"rewake send " + address + " \"the migration is merged\" --notify",
		"rewake send " + address + " - --wait 20",
		"rewake send " + address + " \"also rerun the lint\" --to 8d4ddd85",
	}
}
