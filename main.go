// Command etl-worker runs one-way data transfers between a local
// Obsidian vault and a Notion database.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/yoonsooc/obsidian-notion-etl/internal/cli"

	// The only reference to user-defined plugins: importing the package runs
	// each plugin's init, which registers it into the pipeline registry.
	// Common code (internal/cli) reaches plugins solely via pipeline.Select.
	_ "github.com/yoonsooc/obsidian-notion-etl/plugin"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run dispatches the subcommand and returns its error.
func run(args []string) error {
	if len(args) < 1 {
		cli.FprintUsage(os.Stderr)
		return errors.New("a command is required")
	}

	switch args[0] {
	case "init":
		return cli.RunInit()
	case "migrate":
		return cli.RunMigrate(args[1:])
	case "backup":
		return cli.RunBackup(args[1:])
	case "help", "-h", "--help":
		return cli.RunHelp(args[1:])
	default:
		cli.FprintUsage(os.Stderr)
		return fmt.Errorf("unknown command: %s", args[0])
	}
}
