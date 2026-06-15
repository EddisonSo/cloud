package cli

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// isSSHSession reports whether ec appears to be running over SSH, in which
// case the localhost callback cannot reach the user's browser and we fall
// back to copy-paste only.
func isSSHSession() bool {
	return os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != ""
}

// buildCli2faURL builds the dashboard verification URL. cbPort == 0 omits the
// localhost callback parameter. When cbPort != 0, the state nonce is baked
// into the cb URL so the browser page posts back to the exact address — the
// page needs no extra logic to forward the nonce.
func buildCli2faURL(baseDomain, challenge string, cbPort int, state string) string {
	q := url.Values{}
	q.Set("challenge", challenge)
	if cbPort != 0 {
		q.Set("cb", fmt.Sprintf("http://127.0.0.1:%d?state=%s", cbPort, url.QueryEscape(state)))
	}
	return fmt.Sprintf("https://%s/cli-2fa?%s", baseDomain, q.Encode())
}

// startCallbackListener binds 127.0.0.1 on an OS-assigned port and returns
// the port, a per-attempt random state nonce, a channel that receives the
// first non-empty token POSTed with the correct ?state= param, and a stop
// function. POSTs missing or supplying the wrong state nonce are rejected with
// 403 — this guards against a malicious local page/process injecting a forged
// session token (login-CSRF).
func startCallbackListener() (port int, state string, tokenCh chan string, stop func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, "", nil, nil, err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		_ = ln.Close()
		return 0, "", nil, nil, err
	}
	state = hex.EncodeToString(nonce)
	tokenCh = make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Reject callbacks missing the per-attempt state nonce — guards against a
		// malicious local page/process injecting a forged session token (login-CSRF).
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(state)) != 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
		tok := strings.TrimSpace(string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		if tok != "" {
			select {
			case tokenCh <- tok:
			default:
			}
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	port = ln.Addr().(*net.TCPAddr).Port
	stop = func() { _ = srv.Close() }
	return port, state, tokenCh, stop, nil
}

// awaitToken returns the first token from either channel, or an error on timeout.
func awaitToken(cbCh, pasteCh <-chan string, timeout time.Duration) (string, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case tok := <-cbCh:
			if tok = strings.TrimSpace(tok); tok != "" {
				return tok, nil
			}
		case tok := <-pasteCh:
			if tok = strings.TrimSpace(tok); tok != "" {
				return tok, nil
			}
		case <-timer.C:
			return "", errors.New("timed out waiting for 2FA token")
		}
	}
}
