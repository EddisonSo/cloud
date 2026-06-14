package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

func init() { register(command{name: "registry", run: cmdRegistry}) }

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
func cmdRegistry(c *eddsdk.Client, _ string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: edd registry <repos|tags|rm> [args]")
	}
	ctx := context.Background()
	sub, rest := args[0], args[1:]
	switch sub {
	case "repos":
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
		if len(rest) < 1 {
			return fmt.Errorf("usage: edd registry tags <repo>")
		}
		tags, err := c.ListTags(ctx, rest[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(tags)
		}
		tagTable(os.Stdout, tags)
		return nil
	case "rm":
		if len(rest) != 2 {
			return fmt.Errorf("usage: edd registry rm <repo> <tag>")
		}
		return c.DeleteTag(ctx, rest[0], rest[1])
	default:
		return fmt.Errorf("unknown registry subcommand: %s", sub)
	}
}
