package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"eddisonso.com/edd-cli/pkg/eddsdk"
	"golang.org/x/term"
)

func init() { register(command{name: "auth", run: cmdAuth}) }

// cmdAuth routes: direct actions (login/logout/status/set-token) and resources (service-accounts/tokens).
func cmdAuth(c *eddsdk.Client, cfgPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec auth <login|logout|status|set-token|service-accounts|tokens> [args]")
	}
	switch args[0] {
	case "login":
		return cmdLogin(c, cfgPath, args[1:])
	case "logout":
		return cmdLogout(c, cfgPath, args[1:])
	case "status":
		return cmdStatus(c, cfgPath, args[1:])
	case "set-token":
		return cmdSetToken(c, cfgPath, args[1:])
	case "service-accounts":
		return cmdSA(c, cfgPath, args[1:])
	case "tokens":
		return cmdToken(c, cfgPath, args[1:])
	default:
		return fmt.Errorf("unknown auth action/resource: %s", args[0])
	}
}

func cmdLogin(c *eddsdk.Client, cfgPath string, args []string) error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Username: ")
	user, _ := reader.ReadString('\n')
	user = strings.TrimSpace(user)
	fmt.Print("Password: ")
	pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return err
	}
	res, err := c.Login(context.Background(), user, strings.TrimSpace(string(pwBytes)))
	if err != nil {
		return err
	}
	if res.Requires2FA {
		return complete2FALogin(c, cfgPath, res.ChallengeToken)
	}
	cfg := loadConfig(cfgPath)
	cfg.Token = res.Token
	if cfg.BaseDomain == "" {
		cfg.BaseDomain = "cloud.eddisonso.com"
	}
	if err := saveConfig(cfgPath, cfg); err != nil {
		return err
	}
	fmt.Printf("Logged in as %s\n", res.Username)
	return nil
}

func doLogout(cfgPath string) error {
	cfg := loadConfig(cfgPath)
	cfg.Token = ""
	return saveConfig(cfgPath, cfg)
}

func cmdLogout(c *eddsdk.Client, cfgPath string, args []string) error {
	if err := doLogout(cfgPath); err != nil {
		return err
	}
	if os.Getenv("EC_TOKEN") != "" {
		fmt.Println("Cleared the stored token, but EC_TOKEN is still set in your environment and will keep authenticating you. Run `unset EC_TOKEN` to fully log out.")
		return nil
	}
	fmt.Println("Logged out")
	return nil
}

// isAuthError reports whether err is an authentication/authorization failure
// (401/403) from the API — i.e. the caller is not logged in or lacks access.
func isAuthError(err error) bool {
	var apiErr *eddsdk.APIError
	return errors.As(err, &apiErr) && (apiErr.Status == 401 || apiErr.Status == 403)
}

func cmdStatus(c *eddsdk.Client, cfgPath string, args []string) error {
	s, err := c.Session(context.Background())
	if err != nil {
		if isAuthError(err) {
			fmt.Println("Not logged in.")
			return nil
		}
		return err
	}
	if jsonOutput {
		s.Token = "" // don't leak the bearer token to stdout
		return printJSON(s)
	}
	admin := ""
	if s.IsAdmin {
		admin = " [admin]"
	}
	fmt.Printf("%s (%s)%s\n", s.Username, s.UserID, admin)
	return nil
}

// cmdSetToken stores an ecloud_ service-account token to the config file.
// Token is read from --token flag or stdin. Never echoed to stdout.
func cmdSetToken(c *eddsdk.Client, cfgPath string, args []string) error {
	fs := flag.NewFlagSet("set-token", flag.ContinueOnError)
	tok := fs.String("token", "", "ecloud_ token to store")
	if err := fs.Parse(args); err != nil {
		return err
	}
	token := *tok
	if token == "" {
		fmt.Fprint(os.Stderr, "Token: ")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("reading token: %w", err)
		}
		token = strings.TrimSpace(line)
	}
	if !strings.HasPrefix(token, "ecloud_") {
		return fmt.Errorf("token must start with ecloud_")
	}
	cfg := loadConfig(cfgPath)
	cfg.Token = token
	if cfg.BaseDomain == "" {
		cfg.BaseDomain = "cloud.eddisonso.com"
	}
	if err := saveConfig(cfgPath, cfg); err != nil {
		return err
	}
	fmt.Println("Token saved.")
	return nil
}

// --- service-accounts resource ---

func saTable(w io.Writer, sas []eddsdk.ServiceAccount) {
	rows := make([][]string, len(sas))
	for i, sa := range sas {
		rows[i] = []string{sa.ID, sa.Name, fmt.Sprintf("%d tokens", sa.TokenCount)}
	}
	printTable(w, []string{"ID", "NAME", "TOKENS"}, rows)
}

// cmdSA handles: ec auth service-accounts ls|create|rm
func cmdSA(c *eddsdk.Client, _ string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec auth service-accounts <ls|create|rm> [args]")
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
			return fmt.Errorf("usage: ec auth service-accounts rm <id>")
		}
		return c.DeleteServiceAccount(ctx, rest[0])
	default:
		return fmt.Errorf("unknown service-accounts action: %s", sub)
	}
}

// cmdSACreate parses flags for:
//
//	ec auth service-accounts create --name <n> [--scope <root.uid.res>=<action1,action2>...] [--scopes-json <json>]
func cmdSACreate(ctx context.Context, c *eddsdk.Client, args []string) error {
	fs := flag.NewFlagSet("service-accounts create", flag.ContinueOnError)
	name := fs.String("name", "", "service account name (required)")
	scopesJSON := fs.String("scopes-json", "", `scopes as JSON, e.g. '{"compute.u.containers":["read"]}'`)
	var scopeFlags []string
	fs.Func("scope", `scope as root.uid.resource=action1,action2 (uid may be "me"; repeatable)`, func(s string) error {
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
	scopes, err = expandScopeMe(ctx, c, scopes)
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

// --- tokens resource ---

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

// cmdToken handles: ec auth tokens ls|create|rm
func cmdToken(c *eddsdk.Client, _ string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec auth tokens <ls|create|rm> [args]")
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
			return fmt.Errorf("usage: ec auth tokens rm <id>")
		}
		return c.DeleteToken(ctx, rest[0])
	default:
		return fmt.Errorf("unknown tokens action: %s", sub)
	}
}

// cmdTokenCreate parses flags for:
//
//	ec auth tokens create --name <n> --service-account <id> [--expires-in 30d|90d|365d|never]
//
// Tokens are bound to a service account and inherit its scopes; set scopes on
// the service account (ec auth service-accounts create --scope ...).
func cmdTokenCreate(ctx context.Context, c *eddsdk.Client, args []string) error {
	fs := flag.NewFlagSet("tokens create", flag.ContinueOnError)
	name := fs.String("name", "", "token name (required)")
	sa := fs.String("service-account", "", "service account id the token belongs to (required)")
	expiresIn := fs.String("expires-in", "never", "expiry: 30d|90d|365d|never")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *sa == "" {
		return fmt.Errorf("--service-account is required; tokens inherit a service account's scopes. " +
			"List them with 'ec auth service-accounts ls' or create one with 'ec auth service-accounts create --name ... --scope ...'")
	}
	tok, err := c.CreateServiceAccountToken(ctx, *sa, *name, *expiresIn)
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

// scopesUseMe reports whether any scope key uses the literal "me" in the uid
// position (the second dot-segment), e.g. networking.me.domains.
func scopesUseMe(scopes map[string][]string) bool {
	for k := range scopes {
		if p := strings.SplitN(k, ".", 3); len(p) >= 2 && p[1] == "me" {
			return true
		}
	}
	return false
}

// rewriteScopeMe replaces a literal "me" uid segment with the given uid in
// every scope key (e.g. networking.me.domains -> networking.<uid>.domains).
func rewriteScopeMe(scopes map[string][]string, uid string) map[string][]string {
	out := make(map[string][]string, len(scopes))
	for k, v := range scopes {
		if p := strings.SplitN(k, ".", 3); len(p) >= 2 && p[1] == "me" {
			p[1] = uid
			k = strings.Join(p, ".")
		}
		out[k] = v
	}
	return out
}

// expandScopeMe resolves the "me" uid placeholder against the caller's own
// user id (from the session). It only contacts the API when "me" is actually
// used, so token creation with fully-qualified scopes makes no extra call.
func expandScopeMe(ctx context.Context, c *eddsdk.Client, scopes map[string][]string) (map[string][]string, error) {
	if !scopesUseMe(scopes) {
		return scopes, nil
	}
	s, err := c.Session(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving your user id for 'me' in --scope: %w", err)
	}
	return rewriteScopeMe(scopes, s.UserID), nil
}
