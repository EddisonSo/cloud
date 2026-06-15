package cli

import (
	"fmt"
	"net/url"
	"os"
)

// isSSHSession reports whether ec appears to be running over SSH, in which
// case the localhost callback cannot reach the user's browser and we fall
// back to copy-paste only.
func isSSHSession() bool {
	return os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != ""
}

// buildCli2faURL builds the dashboard verification URL. cbPort == 0 omits the
// localhost callback parameter.
func buildCli2faURL(baseDomain, challenge string, cbPort int) string {
	q := url.Values{}
	q.Set("challenge", challenge)
	if cbPort != 0 {
		q.Set("cb", fmt.Sprintf("http://127.0.0.1:%d", cbPort))
	}
	return fmt.Sprintf("https://%s/cli-2fa?%s", baseDomain, q.Encode())
}
