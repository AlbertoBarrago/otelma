// Command otelma is the CLI entrypoint (pull, run, serve, ps).
package main

import (
	"os"

	"github.com/albz/otelma/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
