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

func init() { register(command{name: "networking", run: cmdNetworking}) }

func domainTable(w io.Writer, domains []eddsdk.Domain) {
	rows := make([][]string, len(domains))
	for i, d := range domains {
		rows[i] = []string{d.ID, strings.Join(d.Zones, ","), d.CreatedAt}
	}
	printTable(w, []string{"ID", "ZONES", "CREATED_AT"}, rows)
}

func mappingTable(w io.Writer, mappings []eddsdk.DomainMapping) {
	rows := make([][]string, len(mappings))
	for i, m := range mappings {
		rows[i] = []string{m.ID, m.Domain, m.ContainerID, strconv.Itoa(m.TargetPort), m.Status}
	}
	printTable(w, []string{"ID", "DOMAIN", "CONTAINER", "PORT", "STATUS"}, rows)
}

// cmdNetworking routes to resources: domains | mappings
func cmdNetworking(c *eddsdk.Client, _ string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec networking <domains|mappings> <action> [args]")
	}
	switch args[0] {
	case "domains":
		return cmdDomains(context.Background(), c, args[1:])
	case "mappings":
		return cmdMappings(context.Background(), c, args[1:])
	default:
		return fmt.Errorf("unknown networking resource: %s", args[0])
	}
}

// cmdDomains handles: ec networking domains ls|add|rm (owned domains / zones)
func cmdDomains(ctx context.Context, c *eddsdk.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec networking domains <ls|add|rm> [args]")
	}
	switch args[0] {
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
		// ec networking domains add <cloudflare-api-token>
		if len(args) != 2 {
			return fmt.Errorf("usage: ec networking domains add <cloudflare-api-token>")
		}
		d, err := c.AddDomain(ctx, args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(d)
		}
		fmt.Printf("added domain %s (zones: %s)\n", d.ID, strings.Join(d.Zones, ", "))
		return nil
	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: ec networking domains rm <id>")
		}
		return c.DeleteDomain(ctx, args[1])
	default:
		return fmt.Errorf("unknown domains action: %s", args[0])
	}
}

// cmdMappings handles: ec networking mappings ls|add|rm (hostname -> container)
func cmdMappings(ctx context.Context, c *eddsdk.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec networking mappings <ls|add|rm> [args]")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "ls":
		mappings, err := c.ListDomainMappings(ctx)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(mappings)
		}
		mappingTable(os.Stdout, mappings)
		return nil
	case "add":
		// ec networking mappings add <container-id> <domain> <port>
		if len(rest) != 3 {
			return fmt.Errorf("usage: ec networking mappings add <container-id> <domain> <port>")
		}
		port, err := strconv.Atoi(rest[2])
		if err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}
		m, err := c.AddDomainMapping(ctx, eddsdk.CreateDomainMappingRequest{
			ContainerID: rest[0], Domain: rest[1], TargetPort: port,
		})
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(m)
		}
		fmt.Printf("added mapping %s (id=%s status=%s)\n", m.Domain, m.ID, m.Status)
		if m.Status == "pending" {
			fmt.Printf("DNS verification: add TXT record %s = %s\n", m.VerifyName, m.VerifyToken)
		}
		return nil
	case "rm":
		if len(rest) < 1 {
			return fmt.Errorf("usage: ec networking mappings rm <id>")
		}
		return c.DeleteDomainMapping(ctx, rest[0])
	default:
		return fmt.Errorf("unknown mappings action: %s", sub)
	}
}
