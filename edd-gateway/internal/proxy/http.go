package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"
)

// handleHTTP handles HTTP connections by extracting the Host header
// and routing to the appropriate container.
func (s *Server) handleHTTP(conn net.Conn) {
	clientAddr := conn.RemoteAddr().String()

	// Read HTTP request line and headers
	reader := bufio.NewReader(conn)

	// Read until we have the complete headers (ends with \r\n\r\n)
	var headerBuf bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			slog.Debug("failed to read HTTP header", "error", err, "client", clientAddr)
			conn.Close()
			return
		}
		headerBuf.WriteString(line)

		// End of headers
		if line == "\r\n" || line == "\n" {
			break
		}

		// Safety limit
		if headerBuf.Len() > 16384 {
			slog.Warn("HTTP headers too large", "client", clientAddr)
			conn.Write([]byte("HTTP/1.1 431 Request Header Fields Too Large\r\n\r\n"))
			conn.Close()
			return
		}
	}

	// Parse Host header
	host := extractHostHeader(headerBuf.String())
	if host == "" {
		slog.Warn("no Host header in HTTP request", "client", clientAddr)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\nCache-Control: no-store, no-cache, must-revalidate\r\nPragma: no-cache\r\n\r\nMissing Host header\r\n"))
		conn.Close()
		return
	}

	// Remove port from host if present
	hostname := host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		hostname = host[:idx]
	}

	// If hostname is an IP address, redirect to the proper hostname
	if ip := net.ParseIP(hostname); ip != nil {
		slog.Debug("IP-based request, redirecting to hostname", "ip", hostname, "client", clientAddr)
		conn.Write([]byte("HTTP/1.1 302 Found\r\nLocation: https://cloud.eddisonso.com/\r\nCache-Control: no-store, no-cache, must-revalidate\r\nPragma: no-cache\r\n\r\n"))
		conn.Close()
		return
	}

	// Get the ingress port from the connection's local address
	ingressPort := 80
	if addr, ok := conn.LocalAddr().(*net.TCPAddr); ok {
		ingressPort = addr.Port
	}
	// Normalize internal ports to external ports
	// HTTP listener uses port 18080 internally for external port 80
	// (keeping 8000-8999 free for container ingress traffic)
	if ingressPort == 18080 {
		ingressPort = 80
	}

	// Extract path from request line
	path := extractRequestPath(headerBuf.String())

	slog.Info("HTTP connection", "host", hostname, "path", path, "port", ingressPort, "client", clientAddr)

	// Try to resolve in order: static routes -> container -> fallback
	var backendAddr string
	var modifiedHeaders []byte

	// 1. Check static routes first
	if route, targetPath, err := s.router.ResolveStaticRoute(hostname, path); err == nil {
		// Redirect HTTP to HTTPS for static routes (core services)
		if ingressPort == 80 {
			requestLine := extractRequestLine(headerBuf.String())
			fullPath := path
			// Preserve query string if present
			if parts := strings.SplitN(requestLine, " ", 3); len(parts) >= 2 {
				if qIdx := strings.Index(parts[1], "?"); qIdx != -1 {
					fullPath = parts[1]
				}
			}
			redirectURL := fmt.Sprintf("https://%s%s", hostname, fullPath)
			slog.Info("HTTP->HTTPS redirect", "host", hostname, "path", path, "location", redirectURL)
			conn.Write([]byte(fmt.Sprintf("HTTP/1.1 301 Moved Permanently\r\nLocation: %s\r\nCache-Control: no-store, no-cache, must-revalidate\r\nPragma: no-cache\r\n\r\n", redirectURL)))
			conn.Close()
			return
		}

		backendAddr = route.Target
		slog.Info(fmt.Sprintf("HTTP %s%s -> %s", hostname, path, route.Target), "targetPath", targetPath, "strip_prefix", route.StripPrefix)

		// If strip_prefix is enabled, rewrite the request path
		if route.StripPrefix && path != targetPath {
			modifiedHeaders = rewriteRequestPath(headerBuf.Bytes(), path, targetPath)
		}
	} else if container, targetPort, err := s.router.ResolveHTTP(hostname, ingressPort); err == nil {
		// 2. Try container routing
		backendAddr = fmt.Sprintf("lb.%s.svc.cluster.local:%d", container.Namespace, targetPort)
		slog.Info(fmt.Sprintf("HTTP %s%s -> %s (container)", hostname, path, backendAddr))
	} else {
		// 3. Fall back to default upstream
		if s.fallbackAddr == "" {
			slog.Warn(fmt.Sprintf("HTTP %s%s -> NO ROUTE", hostname, path), "port", ingressPort)
			conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nCache-Control: no-store, no-cache, must-revalidate\r\nPragma: no-cache\r\n\r\nNo backend available\r\n"))
			conn.Close()
			return
		}
		backendAddr = fmt.Sprintf("%s:%d", s.fallbackAddr, ingressPort)
		slog.Debug(fmt.Sprintf("HTTP %s%s -> %s (fallback)", hostname, path, backendAddr))
	}
	backend, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	if err != nil {
		slog.Error("failed to connect to backend", "host", hostname, "addr", backendAddr, "error", err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nCache-Control: no-store, no-cache, must-revalidate\r\nPragma: no-cache\r\n\r\nBackend connection failed\r\n"))
		conn.Close()
		return
	}

	slog.Debug("proxying HTTP to backend", "host", hostname, "backend", backendAddr)

	// Get any buffered data from the reader
	buffered := make([]byte, reader.Buffered())
	reader.Read(buffered)

	// Use modified headers if path was rewritten, otherwise use original
	headers := headerBuf.Bytes()
	if modifiedHeaders != nil {
		headers = modifiedHeaders
	}

	// Force connection close - gateway doesn't support HTTP keep-alive yet
	headers = addHeader(headers, "Connection", "close")

	// Combine headers with any buffered body data
	initialData := append(headers, buffered...)

	// Extract method from request line for logging
	method := extractMethod(headerBuf.String())

	// Proxy the connection with response logging
	proxyWithResponseLogging(conn, backend, initialData, method, hostname, path, backendAddr)
}

// extractHostHeader finds the Host header value in HTTP headers.
func extractHostHeader(headers string) string {
	lines := strings.Split(headers, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "host:") {
			return strings.TrimSpace(line[5:])
		}
	}
	return ""
}

// extractRequestLine extracts the first line of the HTTP request.
// "GET /foo/bar HTTP/1.1\r\n..." -> "GET /foo/bar HTTP/1.1"
func extractRequestLine(headers string) string {
	idx := strings.Index(headers, "\n")
	if idx == -1 {
		return strings.TrimSpace(headers)
	}
	return strings.TrimSpace(headers[:idx])
}

// extractRequestPath extracts the path from the HTTP request line.
// "GET /foo/bar HTTP/1.1" -> "/foo/bar"
func extractRequestPath(headers string) string {
	// Find the first line (request line)
	idx := strings.Index(headers, "\n")
	if idx == -1 {
		return "/"
	}
	requestLine := strings.TrimSpace(headers[:idx])

	// Parse: METHOD PATH HTTP/VERSION
	parts := strings.SplitN(requestLine, " ", 3)
	if len(parts) < 2 {
		return "/"
	}

	path := parts[1]
	// Remove query string if present
	if qIdx := strings.Index(path, "?"); qIdx != -1 {
		path = path[:qIdx]
	}

	if path == "" {
		return "/"
	}
	return path
}

// rewriteRequestPath replaces the path in the HTTP request line.
func rewriteRequestPath(headers []byte, oldPath, newPath string) []byte {
	headerStr := string(headers)

	// Find and replace in the request line only (first line)
	idx := strings.Index(headerStr, "\n")
	if idx == -1 {
		return headers
	}

	requestLine := headerStr[:idx]
	rest := headerStr[idx:]

	// Replace the path in the request line
	newRequestLine := strings.Replace(requestLine, " "+oldPath+" ", " "+newPath+" ", 1)
	// Also handle case where path might have query string
	newRequestLine = strings.Replace(newRequestLine, " "+oldPath+"?", " "+newPath+"?", 1)

	return []byte(newRequestLine + rest)
}

// addHeader inserts an HTTP header before the final CRLF.
func addHeader(headers []byte, name, value string) []byte {
	headerStr := string(headers)
	// Find the end of headers (double CRLF)
	idx := strings.Index(headerStr, "\r\n\r\n")
	if idx == -1 {
		// Try just \n\n
		idx = strings.Index(headerStr, "\n\n")
		if idx == -1 {
			return headers
		}
		return []byte(headerStr[:idx] + "\n" + name + ": " + value + "\n\n")
	}
	return []byte(headerStr[:idx] + "\r\n" + name + ": " + value + "\r\n\r\n")
}

// extractMethod extracts the HTTP method from the request line.
func extractMethod(headers string) string {
	idx := strings.Index(headers, " ")
	if idx == -1 {
		return "UNKNOWN"
	}
	return headers[:idx]
}

// proxyWithResponseLogging proxies the connection and logs the response status.
func proxyWithResponseLogging(client, backend net.Conn, initialData []byte, method, host, path, backendAddr string) {
	defer client.Close()
	defer backend.Close()

	slog.Info("starting proxy", "method", method, "host", host, "path", path, "backend", backendAddr)

	// Send the request to backend
	if len(initialData) > 0 {
		if _, err := backend.Write(initialData); err != nil {
			slog.Error("failed to write request to backend", "error", err, "host", host, "path", path)
			return
		}
	}

	// Create channels for coordination
	done := make(chan struct{}, 2)

	// Backend -> Client (capture response status)
	go func() {
		defer func() { done <- struct{}{} }()

		// Read response with buffering to capture status line
		buf := make([]byte, 128*1024)
		statusLogged := false

		for {
			n, err := backend.Read(buf)
			if n > 0 {
				// Log response status on first read
				if !statusLogged {
					statusCode, statusText := parseResponseStatus(buf[:n])
					if statusCode >= 400 {
						slog.Warn("HTTP error response",
							"method", method,
							"host", host,
							"path", path,
							"backend", backendAddr,
							"status", statusCode,
							"statusText", statusText,
						)
					} else if statusCode > 0 {
						slog.Info("HTTP response",
							"method", method,
							"host", host,
							"path", path,
							"backend", backendAddr,
							"status", statusCode,
						)
					} else {
						slog.Debug("HTTP response parse failed",
							"method", method,
							"host", host,
							"path", path,
							"backend", backendAddr,
							"firstBytes", string(buf[:minInt(n, 50)]),
						)
					}
					statusLogged = true
				}

				if _, werr := client.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Client -> Backend
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 128*1024)
		for {
			n, err := client.Read(buf)
			if n > 0 {
				if _, werr := backend.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for both directions
	<-done
	<-done
}

// minInt returns the smaller of two integers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// parseResponseStatus extracts the HTTP status code and text from the response.
func parseResponseStatus(data []byte) (int, string) {
	// Look for "HTTP/1.x NNN Status Text\r\n"
	str := string(data)
	idx := strings.Index(str, "\r\n")
	if idx == -1 {
		idx = strings.Index(str, "\n")
	}
	if idx == -1 || idx > 100 {
		return 0, ""
	}

	statusLine := str[:idx]
	// Parse "HTTP/1.1 200 OK" or similar
	parts := strings.SplitN(statusLine, " ", 3)
	if len(parts) < 2 {
		return 0, ""
	}

	var code int
	fmt.Sscanf(parts[1], "%d", &code)

	statusText := ""
	if len(parts) >= 3 {
		statusText = parts[2]
	}

	return code, statusText
}
