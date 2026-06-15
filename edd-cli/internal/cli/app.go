package cli

import (
	"fmt"
	"os"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

// command is a single CLI command (e.g. "compute").
type command struct {
	name string
	run  func(c *eddsdk.Client, cfgPath string, args []string) error
}

var commands = map[string]command{}

func register(cmd command) { commands[cmd.name] = cmd }

var jsonOutput bool

// Run is the CLI entrypoint. Returns the process exit code.
func Run(argv []string) int {
	var flagToken, flagBase string
	args := argv
	for len(args) > 0 && len(args[0]) >= 2 && args[0][:2] == "--" {
		switch {
		case args[0] == "--json":
			jsonOutput = true
			args = args[1:]
		case args[0] == "--token" && len(args) > 1:
			flagToken = args[1]
			args = args[2:]
		case args[0] == "--base" && len(args) > 1:
			flagBase = args[1]
			args = args[2:]
		default:
			fmt.Fprintf(os.Stderr, "unknown global flag: %s\n", args[0])
			return 2
		}
	}
	if len(args) == 0 || args[0] == "help" {
		usage()
		return 2
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		usage()
		return 2
	}
	cfgPath := defaultConfigPath()
	tok, base := resolveToken(flagToken, cfgPath)
	if flagBase != "" {
		base = flagBase
	}
	client := eddsdk.NewClient(eddsdk.Options{BaseDomain: base, Token: tok})
	if err := cmd.run(client, cfgPath, args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, `ec — edd-cloud CLI

Usage: ec [--json] [--token T] [--base D] <category> <resource> <action> [args]

compute containers   ls | get | create | start | stop | rm | logs | pull-policy | ssh | ingress | mounts
compute keys         ls | add | rm

storage namespaces   ls | create | rm
storage files        ls | rm
storage registry     ls | tags <repo> | rm <repo> <tag>

auth login | logout | status | set-token
auth service-accounts ls | create | rm
auth tokens          ls | create | rm

networking domains      ls | add | rm     (owned domains / zones)
networking mappings     ls | add | rm     (hostname -> container)`)
}
