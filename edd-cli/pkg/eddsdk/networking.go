package eddsdk

import "context"

const networkingSvc = "networking"

// ListDomains returns all custom domains for the authenticated user.
// GET /api/domains → {"domains": [...]}
func (c *Client) ListDomains(ctx context.Context) ([]Domain, error) {
	var out struct {
		Domains []Domain `json:"domains"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(networkingSvc), "/api/domains", nil, &out); err != nil {
		return nil, err
	}
	return out.Domains, nil
}

// AddDomain registers a new custom domain for a container.
// POST /api/domains → Domain
func (c *Client) AddDomain(ctx context.Context, req CreateDomainRequest) (*Domain, error) {
	var out Domain
	if err := c.doJSON(ctx, "POST", c.serviceURL(networkingSvc), "/api/domains", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteDomain removes a custom domain by ID.
// DELETE /api/domains/{id} → 204
func (c *Client) DeleteDomain(ctx context.Context, id string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(networkingSvc), "/api/domains/"+id, nil, nil)
}

// ListConnections returns all Cloudflare connections for the authenticated user.
// GET /api/cloudflare-connections → {"connections": [...]}
func (c *Client) ListConnections(ctx context.Context) ([]CloudflareConnection, error) {
	var out struct {
		Connections []CloudflareConnection `json:"connections"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(networkingSvc), "/api/cloudflare-connections", nil, &out); err != nil {
		return nil, err
	}
	return out.Connections, nil
}

// AddConnection adds a new Cloudflare connection using the provided API token.
// POST /api/cloudflare-connections → CloudflareConnection
func (c *Client) AddConnection(ctx context.Context, token string) (*CloudflareConnection, error) {
	var out CloudflareConnection
	body := struct {
		Token string `json:"token"`
	}{Token: token}
	if err := c.doJSON(ctx, "POST", c.serviceURL(networkingSvc), "/api/cloudflare-connections", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteConnection removes a Cloudflare connection by ID.
// DELETE /api/cloudflare-connections/{id} → 204
func (c *Client) DeleteConnection(ctx context.Context, id string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(networkingSvc), "/api/cloudflare-connections/"+id, nil, nil)
}
