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

func repoTable(w io.Writer, repos []eddsdk.Repo) {
	rows := make([][]string, len(repos))
	for i, r := range repos {
		rows[i] = []string{r.Name, strconv.FormatInt(r.TagCount, 10), strconv.FormatInt(r.TotalSize, 10)}
	}
	printTable(w, []string{"NAME", "TAGS", "SIZE"}, rows)
}

func tagTable(w io.Writer, tags []eddsdk.Tag) {
	rows := make([][]string, len(tags))
	for i, t := range tags {
		rows[i] = []string{t.Name, t.Digest[:min(len(t.Digest), 20)], strconv.FormatInt(t.Size, 10)}
	}
	printTable(w, []string{"TAG", "DIGEST", "SIZE"}, rows)
}

func cmdStorage(c *eddsdk.Client, _ string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec storage <namespaces|files|registry> <action> [args]")
	}
	ctx := context.Background()
	switch args[0] {
	case "namespaces":
		return cmdStorageNamespaces(ctx, c, args[1:])
	case "files":
		return cmdStorageFiles(ctx, c, args[1:])
	case "registry":
		return cmdStorageRegistry(ctx, c, args[1:])
	default:
		return fmt.Errorf("unknown storage resource: %s", args[0])
	}
}

func cmdStorageNamespaces(ctx context.Context, c *eddsdk.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec storage namespaces <ls|create|rm> [args]")
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
			return fmt.Errorf("usage: ec storage namespaces create <name>")
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
			return fmt.Errorf("usage: ec storage namespaces rm <name>")
		}
		if err := c.DeleteNamespace(ctx, args[1]); err != nil {
			return err
		}
		fmt.Printf("deleted namespace %s\n", args[1])
		return nil
	default:
		return fmt.Errorf("unknown namespaces action: %s", args[0])
	}
}

func cmdStorageFiles(ctx context.Context, c *eddsdk.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec storage files <ls|rm> [args]")
	}
	switch args[0] {
	case "ls":
		if len(args) < 2 {
			return fmt.Errorf("usage: ec storage files ls <namespace>")
		}
		files, err := c.ListFiles(ctx, args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(files)
		}
		fileTable(os.Stdout, files)
		return nil
	case "rm":
		if len(args) != 3 {
			return fmt.Errorf("usage: ec storage files rm <namespace> <path>")
		}
		return done(c.DeleteFile(ctx, args[1], args[2]), "deleted %s/%s", args[1], args[2])
	default:
		return fmt.Errorf("unknown files action: %s", args[0])
	}
}

func cmdStorageRegistry(ctx context.Context, c *eddsdk.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec storage registry <ls|tags|rm> [args]")
	}
	switch args[0] {
	case "ls":
		repos, err := c.ListRepos(ctx)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(repos)
		}
		repoTable(os.Stdout, repos)
		return nil
	case "tags":
		if len(args) < 2 {
			return fmt.Errorf("usage: ec storage registry tags <repo>")
		}
		tags, err := c.ListTags(ctx, args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(tags)
		}
		tagTable(os.Stdout, tags)
		return nil
	case "rm":
		if len(args) != 3 {
			return fmt.Errorf("usage: ec storage registry rm <repo> <tag>")
		}
		return done(c.DeleteTag(ctx, args[1], args[2]), "deleted %s:%s", args[1], args[2])
	default:
		return fmt.Errorf("unknown registry action: %s", args[0])
	}
}
