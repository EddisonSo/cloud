package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

func init() {
	register(command{name: "sa", run: cmdSA})
	register(command{name: "token", run: cmdToken})
}

func saTable(w io.Writer, sas []eddsdk.ServiceAccount) {
	rows := make([][]string, len(sas))
	for i, sa := range sas {
		rows[i] = []string{sa.ID, sa.Name, fmt.Sprintf("%d tokens", sa.TokenCount)}
	}
	printTable(w, []string{"ID", "NAME", "TOKENS"}, rows)
}

func tokenTable(w io.Writer, toks []eddsdk.Token) {
	rows := make([][]string, len(toks))
	for i, t := range toks {
		exp := "never"
		if t.ExpiresAt > 0 {
			exp = fmt.Sprintf("%d", t.ExpiresAt)
		}
		rows[i] = []string{t.ID, t.Name, exp}
	}
	printTable(w, []string{"ID", "NAME", "EXPIRES_AT"}, rows)
}

// cmdSA handles: edd sa ls|create|rm
func cmdSA(c *eddsdk.Client, _ string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: edd sa <ls|create|rm> [args]")
	}
	ctx := context.Background()
	sub, rest := args[0], args[1:]
	switch sub {
	case "ls":
		sas, err := c.ListServiceAccounts(ctx)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(sas)
		}
		saTable(os.Stdout, sas)
		return nil
	case "create":
		return cmdSACreate(ctx, c, rest)
	case "rm":
		if len(rest) < 1 {
			return fmt.Errorf("usage: edd sa rm <id>")
		}
		return c.DeleteServiceAccount(ctx, rest[0])
	default:
		return fmt.Errorf("unknown sa subcommand: %s", sub)
	}
}

// cmdSACreate parses flags for:
//
//	edd sa create --name <n> [--scope <root.uid.res>=<action1,action2>...] [--scopes-json <json>]
func cmdSACreate(ctx context.Context, c *eddsdk.Client, args []string) error {
	fs := flag.NewFlagSet("sa create", flag.ContinueOnError)
	name := fs.String("name", "", "service account name (required)")
	scopesJSON := fs.String("scopes-json", "", `scopes as JSON, e.g. '{"compute.u.containers":["read"]}'`)
	var scopeFlags []string
	fs.Func("scope", `scope in format root.uid.resource=action1,action2 (repeatable)`, func(s string) error {
		scopeFlags = append(scopeFlags, s)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	scopes, err := parseScopes(scopeFlags, *scopesJSON)
	if err != nil {
		return err
	}
	sa, err := c.CreateServiceAccount(ctx, *name, scopes)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(sa)
	}
	fmt.Printf("created service account %s (%s)\n", sa.Name, sa.ID)
	return nil
}

// cmdToken handles: edd token ls|create|rm
func cmdToken(c *eddsdk.Client, _ string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: edd token <ls|create|rm> [args]")
	}
	ctx := context.Background()
	sub, rest := args[0], args[1:]
	switch sub {
	case "ls":
		toks, err := c.ListTokens(ctx)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toks)
		}
		tokenTable(os.Stdout, toks)
		return nil
	case "create":
		return cmdTokenCreate(ctx, c, rest)
	case "rm":
		if len(rest) < 1 {
			return fmt.Errorf("usage: edd token rm <id>")
		}
		// NOTE: DELETE /api/tokens/{id} is not currently registered in the auth service.
		return c.DeleteToken(ctx, rest[0])
	default:
		return fmt.Errorf("unknown token subcommand: %s", sub)
	}
}

// cmdTokenCreate parses flags for:
//
//	edd token create --name <n> --expires-in <30d|90d|365d|never> [--scope ...] [--scopes-json ...]
func cmdTokenCreate(ctx context.Context, c *eddsdk.Client, args []string) error {
	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	name := fs.String("name", "", "token name (required)")
	expiresIn := fs.String("expires-in", "never", "expiry: 30d|90d|365d|never")
	scopesJSON := fs.String("scopes-json", "", `scopes as JSON`)
	var scopeFlags []string
	fs.Func("scope", `scope in format root.uid.resource=action1,action2 (repeatable)`, func(s string) error {
		scopeFlags = append(scopeFlags, s)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	scopes, err := parseScopes(scopeFlags, *scopesJSON)
	if err != nil {
		return err
	}
	tok, err := c.CreateToken(ctx, eddsdk.CreateTokenRequest{
		Name: *name, Scopes: scopes, ExpiresIn: *expiresIn,
	})
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(tok)
	}
	fmt.Printf("created token %s (%s)\n", tok.Name, tok.ID)
	if tok.Token != "" {
		fmt.Printf("token: %s\n", tok.Token)
		fmt.Println("(save this — it will not be shown again)")
	}
	return nil
}

// parseScopes combines --scope flags and/or a --scopes-json value into a map.
// --scope format: root.uid.resource=action1,action2
func parseScopes(flags []string, raw string) (map[string][]string, error) {
	scopes := map[string][]string{}
	for _, f := range flags {
		idx := strings.Index(f, "=")
		if idx <= 0 {
			return nil, fmt.Errorf("invalid --scope %q: expected key=action1,action2", f)
		}
		key := f[:idx]
		actions := strings.Split(f[idx+1:], ",")
		scopes[key] = append(scopes[key], actions...)
	}
	if raw != "" {
		var extra map[string][]string
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			return nil, fmt.Errorf("invalid --scopes-json: %w", err)
		}
		for k, v := range extra {
			scopes[k] = append(scopes[k], v...)
		}
	}
	return scopes, nil
}
