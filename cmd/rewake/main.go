// Command rewake lets coding agents running on this machine message each other.
package main

import (
	"os"

	"github.com/iiiokojiadbi/rewake/internal/cli"
	// The catalogue registers every harness rewake can run.
	_ "github.com/iiiokojiadbi/rewake/internal/harness/catalog"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
