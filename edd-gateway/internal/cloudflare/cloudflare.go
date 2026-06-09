// Package cloudflare is a minimal Cloudflare v4 API client used to automate
// custom-domain DNS with a USER-provided token: list zones, upsert a
// grey-cloud CNAME, and clean up on delete. proxied:false is forced — a
// proxied (orange-cloud) record breaks ACME TLS-ALPN-01 and causes an
// HTTP->HTTPS redirect loop.
package cloudflare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrZoneNotFound means no zone on the token covers the domain — the caller
// should fall back to the manual verification flow.
var ErrZoneNotFound = errors.New("cloudflare: zone not found for domain")

// Zone is a Cloudflare zone visible to the token.
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Client talks to the Cloudflare v4 API with a bearer token.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// New builds a client for api.cloudflare.com using the given (user) token.
func New(token string) *Client {
	return &Client{
		token:   token,
		baseURL: "https://api.cloudflare.com/client/v4",
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// NewForTest builds a client against a stub server. Test use only.
func NewForTest(token, baseURL string, h *http.Client) *Client {
	return &Client{token: token, baseURL: baseURL, http: h}
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Errors  []apiError      `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

func (c *Client) do(method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("cloudflare: marshal body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("cloudflare: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	var env apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("cloudflare: decode response (%s): %w", resp.Status, err)
	}
	if !env.Success {
		msg := "unknown error"
		if len(env.Errors) > 0 {
			msg = fmt.Sprintf("%d: %s", env.Errors[0].Code, env.Errors[0].Message)
		}
		return fmt.Errorf("cloudflare: API error: %s", msg)
	}
	if out != nil {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("cloudflare: decode result: %w", err)
		}
	}
	return nil
}

// ListZones returns the zones the token can see (also used to validate a
// token at save time).
func (c *Client) ListZones() ([]Zone, error) {
	var zones []Zone
	if err := c.do("GET", "/zones?per_page=50", nil, &zones); err != nil {
		return nil, err
	}
	return zones, nil
}

// FindZone returns the zone ID whose name is the longest dot-suffix match
// for domain, or ErrZoneNotFound.
func (c *Client) FindZone(domain string) (string, error) {
	zones, err := c.ListZones()
	if err != nil {
		return "", err
	}
	id, ok := matchZone(domain, zones)
	if !ok {
		return "", ErrZoneNotFound
	}
	return id, nil
}

func matchZone(domain string, zones []Zone) (string, bool) {
	bestID, bestLen := "", 0
	for _, z := range zones {
		if (domain == z.Name || strings.HasSuffix(domain, "."+z.Name)) && len(z.Name) > bestLen {
			bestID, bestLen = z.ID, len(z.Name)
		}
	}
	return bestID, bestLen > 0
}

type dnsRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"` // 1 = automatic
}

func (c *Client) findRecord(zoneID, name string) (*dnsRecord, error) {
	var recs []dnsRecord
	path := "/zones/" + zoneID + "/dns_records?name=" + url.QueryEscape(name)
	if err := c.do("GET", path, nil, &recs); err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	return &recs[0], nil
}

// UpsertCNAME creates or rewrites the record for name as an unproxied
// (grey-cloud) CNAME to target. Overwriting deliberately repairs a
// misconfigured proxied record.
func (c *Client) UpsertCNAME(zoneID, name, target string) error {
	body := dnsRecord{Type: "CNAME", Name: name, Content: target, Proxied: false, TTL: 1}
	existing, err := c.findRecord(zoneID, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return c.do("POST", "/zones/"+zoneID+"/dns_records", body, nil)
	}
	return c.do("PUT", "/zones/"+zoneID+"/dns_records/"+existing.ID, body, nil)
}

// DeleteRecord removes the record for name ONLY if its content matches
// expectedContent — never destroys a record the user repurposed.
func (c *Client) DeleteRecord(zoneID, name, expectedContent string) error {
	existing, err := c.findRecord(zoneID, name)
	if err != nil {
		return err
	}
	if existing == nil || existing.Content != expectedContent {
		return nil
	}
	return c.do("DELETE", "/zones/"+zoneID+"/dns_records/"+existing.ID, nil, nil)
}
