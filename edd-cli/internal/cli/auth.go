package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"eddisonso.com/edd-cli/pkg/eddsdk"
	"golang.org/x/term"
)

func init() {
	register(command{name: "login", run: cmdLogin})
	register(command{name: "logout", run: cmdLogout})
	register(command{name: "whoami", run: cmdWhoami})
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
		return fmt.Errorf("this account requires 2FA/WebAuthn, which the CLI can't do interactively; create a service-account token in the dashboard and set EDD_TOKEN instead")
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
	fmt.Println("Logged out")
	return nil
}

func cmdWhoami(c *eddsdk.Client, cfgPath string, args []string) error {
	s, err := c.Session(context.Background())
	if err != nil {
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
