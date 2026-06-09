package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"eddisonso.com/edd-gateway/internal/router"
)

// Server handles TCP proxying with protocol detection.
type Server struct {
	router       *router.Router
	fallbackAddr string // fallback upstream for non-container traffic (e.g., "192.168.3.150")
	listeners    []net.Listener
	mu           sync.Mutex
	closed       bool
	tlsConfig    *tls.Config // TLS config for termination
	wildcardCert *tls.Certificate
	onDemand     func(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

// NewServer creates a new proxy server.
func NewServer(r *router.Router, fallbackAddr string) *Server {
	return &Server{
		router:       r,
		fallbackAddr: fallbackAddr,
	}
}

// LoadTLSCert loads the wildcard TLS certificate used for platform domains and
// installs a GetCertificate callback (custom domains are served on-demand).
func (s *Server) LoadTLSCert(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("load TLS cert: %w", err)
	}
	s.wildcardCert = &cert
	s.tlsConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		// acme-tls/1 first so TLS-ALPN-01 challenges are negotiated; http/1.1 for normal traffic.
		NextProtos:     []string{"acme-tls/1", "http/1.1"},
		GetCertificate: s.getCertificate,
	}
	slog.Info("loaded TLS certificate", "cert", certFile)
	return nil
}

// EnableOnDemandTLS installs certmagic's on-demand certificate callback for
// non-platform (custom) domains.
func (s *Server) EnableOnDemandTLS(onDemand func(*tls.ClientHelloInfo) (*tls.Certificate, error)) {
	s.onDemand = onDemand
}

// getCertificate serves the wildcard cert for platform domains and delegates
// custom domains to certmagic (which also answers acme-tls/1 challenges).
func (s *Server) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := strings.ToLower(hello.ServerName)
	// Serve the wildcard cert for platform domains and as the fallback for an
	// empty SNI; only delegate real custom domains to on-demand issuance.
	if name != "" && s.onDemand != nil && !isPlatformDomain(name) {
		return s.onDemand(hello)
	}
	return s.wildcardCert, nil
}

// isPlatformDomain reports whether a name is covered by the wildcard cert.
func isPlatformDomain(name string) bool {
	return name == "eddisonso.com" || strings.HasSuffix(name, ".eddisonso.com")
}

// ListenSSH starts the SSH proxy listener.
func (s *Server) ListenSSH(port int) error {
	return s.listen(port, s.handleSSH)
}

// ListenHTTP starts the HTTP proxy listener.
func (s *Server) ListenHTTP(port int) error {
	return s.listen(port, s.handleHTTP)
}

// ListenTLS starts the TLS/HTTPS proxy listener.
func (s *Server) ListenTLS(port int) error {
	return s.listen(port, s.handleTLS)
}

// ListenMulti starts a multi-protocol listener that auto-detects SSH/HTTP/TLS.
func (s *Server) ListenMulti(port int) error {
	return s.listen(port, s.handleMulti)
}

// handleMulti detects the protocol from the first bytes and routes accordingly.
func (s *Server) handleMulti(conn net.Conn) {
	// Read first few bytes to detect protocol
	buf := make([]byte, 8)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		slog.Debug("failed to read protocol detection bytes", "error", err)
		conn.Close()
		return
	}
	buf = buf[:n]

	// Wrap connection to replay the peeked bytes
	peekedConn := &peekedConn{Conn: conn, peeked: buf}

	// Detect protocol
	switch {
	case n >= 4 && string(buf[:4]) == "SSH-":
		slog.Debug("detected SSH protocol")
		s.handleSSH(peekedConn)
	case n >= 1 && buf[0] == 0x16:
		slog.Debug("detected TLS protocol")
		s.handleTLSWithPeek(peekedConn, buf)
	case isHTTPMethod(buf):
		slog.Debug("detected HTTP protocol")
		s.handleHTTPWithPeek(peekedConn, buf)
	default:
		slog.Warn("unknown protocol", "bytes", buf)
		conn.Close()
	}
}

// isHTTPMethod checks if the bytes start with an HTTP method.
func isHTTPMethod(buf []byte) bool {
	methods := []string{"GET ", "POST", "PUT ", "HEAD", "DELE", "OPTI", "PATC", "CONN", "TRAC"}
	if len(buf) < 4 {
		return false
	}
	prefix := string(buf[:4])
	for _, m := range methods {
		if prefix == m {
			return true
		}
	}
	return false
}

// peekedConn wraps a net.Conn to replay peeked bytes on first read.
type peekedConn struct {
	net.Conn
	peeked []byte
	offset int
}

func (c *peekedConn) Read(b []byte) (int, error) {
	if c.offset < len(c.peeked) {
		n := copy(b, c.peeked[c.offset:])
		c.offset += n
		return n, nil
	}
	return c.Conn.Read(b)
}

// handleTLSWithPeek handles TLS with already-peeked bytes.
func (s *Server) handleTLSWithPeek(conn net.Conn, peeked []byte) {
	// The peekedConn will replay the peeked bytes, so just call the normal handler
	s.handleTLS(conn)
}

// handleHTTPWithPeek handles HTTP with already-peeked bytes.
func (s *Server) handleHTTPWithPeek(conn net.Conn, peeked []byte) {
	// The peekedConn will replay the peeked bytes, so just call the normal handler
	s.handleHTTP(conn)
}

func (s *Server) listen(port int, handler func(net.Conn)) error {
	ln, err := net.Listen("tcp", formatAddr(port))
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.listeners = append(s.listeners, ln)
	s.mu.Unlock()

	slog.Info("listening", "port", port)

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			slog.Error("accept failed", "error", err)
			continue
		}

		go handler(conn)
	}
}

// Close shuts down all listeners.
func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	for _, ln := range s.listeners {
		ln.Close()
	}
	s.mu.Unlock()
}

// proxy copies data bidirectionally between client and backend.
func proxy(client, backend net.Conn, initialData []byte) {
	defer client.Close()
	defer backend.Close()

	// Send any initial data that was read during protocol detection
	if len(initialData) > 0 {
		if _, err := backend.Write(initialData); err != nil {
			slog.Error("failed to write initial data", "error", err)
			return
		}
	}

	// Bidirectional copy
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(backend, client)
		if tc, ok := backend.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	go func() {
		io.Copy(client, backend)
		if tc, ok := client.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	// Wait for both directions to complete
	<-done
	<-done
}

// dialBackend connects to the container's backend service.
func (s *Server) dialBackend(ip string, port int) (net.Conn, error) {
	addr := net.JoinHostPort(ip, formatPort(port))
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func formatAddr(port int) string {
	return fmt.Sprintf(":%d", port)
}

func formatPort(port int) string {
	return fmt.Sprintf("%d", port)
}
