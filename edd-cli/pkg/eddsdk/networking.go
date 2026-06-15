package eddsdk

import "context"

const networkingSvc = "networking"

// ListDomains returns all owned domains/zones for the authenticated user.
// GET /api/domains → {"connections": [...]}
func (c *Client) ListDomains(ctx context.Context) ([]Domain, error) {
	var out struct {
		Connections []Domain `json:"connections"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(networkingSvc), "/api/domains", nil, &out); err != nil {
		return nil, err
	}
	return out.Connections, nil
}

// AddDomain registers an owned domain using the provided Cloudflare API token.
// POST /api/domains {"token": token} → Domain
func (c *Client) AddDomain(ctx context.Context, token string) (*Domain, error) {
	var out Domain
	body := struct {
		Token string `json:"token"`
	}{Token: token}
	if err := c.doJSON(ctx, "POST", c.serviceURL(networkingSvc), "/api/domains", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteDomain removes an owned domain by ID.
// DELETE /api/domains/{id} → 204
func (c *Client) DeleteDomain(ctx context.Context, id string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(networkingSvc), "/api/domains/"+id, nil, nil)
}

// RefreshDomain re-snapshots an owned domain's zones with its stored token.
// POST /api/domains/{id}/refresh → Domain
func (c *Client) RefreshDomain(ctx context.Context, id string) (*Domain, error) {
	var out Domain
	if err := c.doJSON(ctx, "POST", c.serviceURL(networkingSvc), "/api/domains/"+id+"/refresh", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListDomainMappings returns all hostname->container mappings for the authenticated user.
// GET /api/domain-mappings → {"domains": [...]}
func (c *Client) ListDomainMappings(ctx context.Context) ([]DomainMapping, error) {
	var out struct {
		Domains []DomainMapping `json:"domains"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(networkingSvc), "/api/domain-mappings", nil, &out); err != nil {
		return nil, err
	}
	return out.Domains, nil
}

// AddDomainMapping registers a new hostname->container mapping.
// POST /api/domain-mappings → DomainMapping
func (c *Client) AddDomainMapping(ctx context.Context, req CreateDomainMappingRequest) (*DomainMapping, error) {
	var out DomainMapping
	if err := c.doJSON(ctx, "POST", c.serviceURL(networkingSvc), "/api/domain-mappings", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteDomainMapping removes a hostname->container mapping by ID.
// DELETE /api/domain-mappings/{id} → 204
func (c *Client) DeleteDomainMapping(ctx context.Context, id string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(networkingSvc), "/api/domain-mappings/"+id, nil, nil)
}
