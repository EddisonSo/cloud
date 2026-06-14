package main

import (
	"os"

	"eddisonso.com/edd-cli/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:])) }
