package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

func init() {
	register(command{name: "domains", run: cmdDomains})
	register(command{name: "net", run: cmdNet})
}

func domainTable(w io.Writer, domains []eddsdk.Domain) {
	rows := make([][]string, len(domains))
	for i, d := range domains {
		rows[i] = []string{d.ID, d.Domain, d.ContainerID, strconv.Itoa(d.TargetPort), d.Status}
	}
	printTable(w, []string{"ID", "DOMAIN", "CONTAINER", "PORT", "STATUS"}, rows)
}

func connTable(w io.Writer, conns []eddsdk.CloudflareConnection) {
	rows := make([][]string, len(conns))
	for i, c := range conns {
		rows[i] = []string{c.ID, strings.Join(c.Zones, ","), c.CreatedAt}
	}
	printTable(w, []string{"ID", "ZONES", "CREATED_AT"}, rows)
}

// cmdDomains handles: edd domains ls|add|rm
func cmdDomains(c *eddsdk.Client, _ string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: edd domains <ls|add|rm> [args]")
	}
	ctx := context.Background()
	sub, rest := args[0], args[1:]
	switch sub {
	case "ls":
		domains, err := c.ListDomains(ctx)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(domains)
		}
		domainTable(os.Stdout, domains)
		return nil
	case "add":
		// edd domains add <container-id> <domain> <port>
		if len(rest) != 3 {
			return fmt.Errorf("usage: edd domains add <container-id> <domain> <port>")
		}
		port, err := strconv.Atoi(rest[2])
		if err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}
		d, err := c.AddDomain(ctx, eddsdk.CreateDomainRequest{
			ContainerID: rest[0], Domain: rest[1], TargetPort: port,
		})
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(d)
		}
		fmt.Printf("added domain %s (id=%s status=%s)\n", d.Domain, d.ID, d.Status)
		if d.Status == "pending" {
			fmt.Printf("DNS verification: add TXT record %s = %s\n", d.VerifyName, d.VerifyToken)
		}
		return nil
	case "rm":
		if len(rest) < 1 {
			return fmt.Errorf("usage: edd domains rm <id>")
		}
		return c.DeleteDomain(ctx, rest[0])
	default:
		return fmt.Errorf("unknown domains subcommand: %s", sub)
	}
}

// cmdNet handles: edd net connections ls|add|rm
func cmdNet(c *eddsdk.Client, _ string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: edd net connections <ls|add|rm> [args]")
	}
	sub, rest := args[0], args[1:]
	if sub != "connections" {
		return fmt.Errorf("unknown net subcommand: %s (try: connections)", sub)
	}
	return cmdNetConnections(context.Background(), c, rest)
}

func cmdNetConnections(ctx context.Context, c *eddsdk.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: edd net connections <ls|add|rm> [args]")
	}
	switch args[0] {
	case "ls":
		conns, err := c.ListConnections(ctx)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(conns)
		}
		connTable(os.Stdout, conns)
		return nil
	case "add":
		// edd net connections add <cloudflare-api-token>
		if len(args) != 2 {
			return fmt.Errorf("usage: edd net connections add <cloudflare-api-token>")
		}
		conn, err := c.AddConnection(ctx, args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(conn)
		}
		fmt.Printf("added Cloudflare connection %s (zones: %s)\n", conn.ID, strings.Join(conn.Zones, ", "))
		return nil
	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: edd net connections rm <id>")
		}
		return c.DeleteConnection(ctx, args[1])
	default:
		return fmt.Errorf("unknown net connections subcommand: %s", args[0])
	}
}
