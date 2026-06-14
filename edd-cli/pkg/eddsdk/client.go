package eddsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Options configures a Client.
type Options struct {
	BaseDomain string       // default "cloud.eddisonso.com"
	Token      string       // bearer JWT (session or ecloud_ SA token)
	HTTPClient *http.Client // optional
}

// Client is a typed client for edd-cloud services.
type Client struct {
	baseDomain  string
	token       string
	http        *http.Client
	urlOverride string // test hook; when set, serviceURL returns it for any service
}

func NewClient(o Options) *Client {
	base := o.BaseDomain
	if base == "" {
		base = "cloud.eddisonso.com"
	}
	hc := o.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseDomain: base, token: o.Token, http: hc}
}

// serviceSubdomain maps a logical service name to its subdomain prefix.
func serviceSubdomain(svc string) string {
	if svc == "networking" {
		return "net"
	}
	return svc
}

func (c *Client) serviceURL(svc string) string {
	if c.urlOverride != "" {
		return c.urlOverride
	}
	return fmt.Sprintf("https://%s.%s", serviceSubdomain(svc), c.baseDomain)
}

// doJSON performs a JSON request to baseURL+path. body (if non-nil) is JSON-encoded;
// out (if non-nil) receives the decoded JSON response. Non-2xx -> *APIError.
func (c *Client) doJSON(ctx context.Context, method, baseURL, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(data))}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
