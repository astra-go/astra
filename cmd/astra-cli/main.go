// astra-cli is the interactive CLI scaffolding tool for the Astra framework.
package main

import (
	"fmt"
	"os"

	"github.com/astra-go/astra/cmd/astra-cli/cmd"
)

const version = "0.1.0"

func main() {
	root := cmd.NewRootCmd(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
