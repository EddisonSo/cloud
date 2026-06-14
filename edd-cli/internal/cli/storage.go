package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

func init() { register(command{name: "storage", run: cmdStorage}) }

func nsTable(w io.Writer, nss []eddsdk.Namespace) {
	rows := make([][]string, len(nss))
	for i, ns := range nss {
		rows[i] = []string{ns.Name, strconv.Itoa(ns.Count), strconv.Itoa(ns.Visibility)}
	}
	printTable(w, []string{"NAME", "FILES", "VISIBILITY"}, rows)
}

func fileTable(w io.Writer, files []eddsdk.FileInfo) {
	rows := make([][]string, len(files))
	for i, f := range files {
		rows[i] = []string{f.Name, f.Namespace, strconv.FormatUint(f.Size, 10)}
	}
	printTable(w, []string{"NAME", "NAMESPACE", "SIZE"}, rows)
}

func cmdStorage(c *eddsdk.Client, _ string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: edd storage <ns|ls|rm> [args]")
	}
	ctx := context.Background()
	sub, rest := args[0], args[1:]
	switch sub {
	case "ns":
		return cmdStorageNS(ctx, c, rest)
	case "ls":
		// list files: edd storage ls <namespace>
		if len(rest) < 1 {
			return fmt.Errorf("usage: edd storage ls <namespace>")
		}
		files, err := c.ListFiles(ctx, rest[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(files)
		}
		fileTable(os.Stdout, files)
		return nil
	case "rm":
		// delete file: edd storage rm <namespace> <path>
		if len(rest) != 2 {
			return fmt.Errorf("usage: edd storage rm <namespace> <path>")
		}
		return c.DeleteFile(ctx, rest[0], rest[1])
	default:
		return fmt.Errorf("unknown storage subcommand: %s", sub)
	}
}

func cmdStorageNS(ctx context.Context, c *eddsdk.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: edd storage ns <ls|create|rm> [args]")
	}
	switch args[0] {
	case "ls":
		nss, err := c.ListNamespaces(ctx)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(nss)
		}
		nsTable(os.Stdout, nss)
		return nil
	case "create":
		if len(args) < 2 {
			return fmt.Errorf("usage: edd storage ns create <name>")
		}
		ns, err := c.CreateNamespace(ctx, args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(ns)
		}
		fmt.Printf("created namespace %s\n", ns.Name)
		return nil
	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: edd storage ns rm <name>")
		}
		if err := c.DeleteNamespace(ctx, args[1]); err != nil {
			return err
		}
		fmt.Printf("deleted namespace %s\n", args[1])
		return nil
	default:
		return fmt.Errorf("unknown storage ns subcommand: %s", args[0])
	}
}
