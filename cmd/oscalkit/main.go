// Command oscalkit resolves OSCAL profiles, validates OSCAL documents,
// generates SSP skeletons from component definitions and reports control
// gaps in SSPs against their baselines.
package main

import (
	"os"

	"github.com/SlateGitOrg/oscalkit/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
