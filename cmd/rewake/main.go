// Command rewake lets coding agents running on this machine message each other.
package main

import (
	"os"

	"github.com/praline-labs/rewake/internal/adapter/catalog"
	"github.com/praline-labs/rewake/internal/cli"
)

// The binary is the one place that knows which adapters exist: it builds the
// catalog and hands it to the CLI as a value.
func main() {
	os.Exit(cli.Run(catalog.New(), os.Args[1:], os.Stdout, os.Stderr))
}
