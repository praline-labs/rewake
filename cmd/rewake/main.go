// Command rewake lets coding agents running on this machine message each other.
package main

import (
	"os"

	"github.com/praline-labs/rewake/internal/cli"
	// The catalog registers every harness rewake can run.
	_ "github.com/praline-labs/rewake/internal/harness/catalog"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
