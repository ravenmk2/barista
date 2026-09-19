package main

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"

	"barista/internal/cli"
)

var version = "dev"

func main() {
	if err := fang.Execute(context.Background(), cli.NewRootCmd(version), fang.WithVersion(version)); err != nil {
		os.Exit(2)
	}
	os.Exit(cli.ExitCode)
}
