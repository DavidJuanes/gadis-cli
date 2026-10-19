// Command gadis is a CLI for gadisline.com, the online shop of Gadis supermarkets.
package main

import (
	"os"

	"github.com/DavidJuanes/gadis-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
