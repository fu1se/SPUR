// Command app is the composition root: it wires concrete adapter/infra
// implementations into use cases and hands control to the CLI. No business
// logic lives here.
package main

import (
	"os"

	"github.com/fu1se/localizator/internal/adapter/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
