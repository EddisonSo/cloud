package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || isAllowedOrigin(origin)
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// HandleTerminal handles WebSocket connections for cloud terminal
func (h *Handler) HandleTerminal(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("id")
	if containerID == "" {
		http.Error(w, "container ID required", http.StatusBadRequest)
		return
	}

	userID, _, _ := getUserFromContext(r.Context())

	// Verify user owns container
	container, err := h.db.GetContainer(containerID)
	if err != nil {
		slog.Error("failed to get container", "error", err, "container", containerID)
		http.Error(w, "container not found", http.StatusNotFound)
		return
	}

	if container.UserID != userID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if container.Status != "running" {
		http.Error(w, "container not running", http.StatusBadRequest)
		return
	}

	if !container.ExternalIP.Valid || container.ExternalIP.String == "" {
		http.Error(w, "container has no external IP", http.StatusBadRequest)
		return
	}

	// Upgrade to WebSocket
	ws, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}
	defer ws.Close()

	// Configure WebSocket keepalive
	ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	slog.Info("terminal session started", "container", containerID, "user", userID)

	// Generate temporary keypair
	pubKey, privKey, err := generateTempKeypair()
	if err != nil {
		slog.Error("failed to generate temp keypair", "error", err)
		ws.WriteMessage(websocket.TextMessage, []byte("error: failed to generate keypair"))
		return
	}

	// Inject public key into container
	keyID := fmt.Sprintf("terminal-%d", time.Now().UnixNano())

	if err := h.k8s.InjectTempKey(r.Context(), container.Namespace, pubKey, keyID); err != nil {
		slog.Error("failed to inject temp key", "error", err, "container", containerID)
		ws.WriteMessage(websocket.TextMessage, []byte("error: failed to setup terminal"))
		return
	}

	// Small delay for daemon to pick up the key
	time.Sleep(200 * time.Millisecond)

	// Use K8s service DNS for reliable cross-node connectivity
	sshHost := fmt.Sprintf("lb.%s.svc.cluster.local", container.Namespace)

	// Connect to container via SSH using K8s service DNS
	sshClient, err := dialSSH(sshHost, 22, "root", privKey)
	if err != nil {
		slog.Error("failed to SSH to container", "error", err, "container", containerID, "host", sshHost)
		ws.WriteMessage(websocket.TextMessage, []byte("error: failed to connect to container"))
		return
	}
	defer sshClient.Close()

	// Open session and PTY
	session, err := sshClient.NewSession()
	if err != nil {
		slog.Error("failed to create SSH session", "error", err)
		ws.WriteMessage(websocket.TextMessage, []byte("error: failed to create session"))
		return
	}
	defer session.Close()

	// Request PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		slog.Error("failed to request PTY", "error", err)
		ws.WriteMessage(websocket.TextMessage, []byte("error: failed to allocate terminal"))
		return
	}

	// Get stdin/stdout pipes
	stdin, err := session.StdinPipe()
	if err != nil {
		slog.Error("failed to get stdin pipe", "error", err)
		return
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		slog.Error("failed to get stdout pipe", "error", err)
		return
	}

	// Start shell
	if err := session.Shell(); err != nil {
		slog.Error("failed to start shell", "error", err)
		ws.WriteMessage(websocket.TextMessage, []byte("error: failed to start shell"))
		return
	}

	slog.Info("terminal connected", "container", containerID)

	// Proxy between WebSocket and SSH
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var wg sync.WaitGroup

	// WebSocket keepalive ping - ping every 5 seconds to keep connection alive through proxies
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
					slog.Debug("ping failed", "error", err)
					cancel()
					return
				}
			}
		}
	}()

	// WebSocket -> SSH (stdin)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stdin.Close()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			_, message, err := ws.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					slog.Debug("websocket read error", "error", err)
				}
				cancel()
				return
			}

			if _, err := stdin.Write(message); err != nil {
				slog.Debug("stdin write error", "error", err)
				cancel()
				return
			}
		}
	}()

	// SSH (stdout) -> WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := stdout.Read(buf)
			if err != nil {
				if err != io.EOF {
					slog.Debug("stdout read error", "error", err)
				}
				cancel()
				return
			}

			if err := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				slog.Debug("websocket write error", "error", err)
				cancel()
				return
			}
		}
	}()

	// Wait for session to end
	session.Wait()
	cancel()
	wg.Wait()

	slog.Info("terminal session ended", "container", containerID)
}

// generateTempKeypair creates a temporary ed25519 keypair
func generateTempKeypair() (pubKeyStr string, signer ssh.Signer, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("generate key: %w", err)
	}

	signer, err = ssh.NewSignerFromKey(priv)
	if err != nil {
		return "", nil, fmt.Errorf("create signer: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", nil, fmt.Errorf("create public key: %w", err)
	}

	// Format as authorized_keys line
	pubKeyStr = fmt.Sprintf("%s %s temp-terminal-key",
		sshPub.Type(),
		base64.StdEncoding.EncodeToString(sshPub.Marshal()))

	return pubKeyStr, signer, nil
}

// dialSSH connects to an SSH server using the provided private key
func dialSSH(host string, port int, user string, signer ssh.Signer) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	return ssh.Dial("tcp", addr, config)
}
