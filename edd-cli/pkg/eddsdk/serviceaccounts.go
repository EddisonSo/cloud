package eddsdk

import "context"

// ListServiceAccounts returns all service accounts for the authenticated user.
// GET /api/service-accounts → []ServiceAccount
func (c *Client) ListServiceAccounts(ctx context.Context) ([]ServiceAccount, error) {
	var out []ServiceAccount
	if err := c.doJSON(ctx, "GET", c.serviceURL(authSvc), "/api/service-accounts", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateServiceAccount creates a new service account.
// POST /api/service-accounts → ServiceAccount
func (c *Client) CreateServiceAccount(ctx context.Context, name string, scopes map[string][]string) (*ServiceAccount, error) {
	var out ServiceAccount
	body := struct {
		Name   string              `json:"name"`
		Scopes map[string][]string `json:"scopes"`
	}{Name: name, Scopes: scopes}
	if err := c.doJSON(ctx, "POST", c.serviceURL(authSvc), "/api/service-accounts", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteServiceAccount deletes a service account by ID.
// DELETE /api/service-accounts/{id} → {"status":"ok"}
func (c *Client) DeleteServiceAccount(ctx context.Context, id string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(authSvc), "/api/service-accounts/"+id, nil, nil)
}

// ListTokens returns all API tokens for the authenticated user.
// GET /api/tokens → []Token
func (c *Client) ListTokens(ctx context.Context) ([]Token, error) {
	var out []Token
	if err := c.doJSON(ctx, "GET", c.serviceURL(authSvc), "/api/tokens", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateToken creates a new API token. The returned Token.Token field holds the
// raw ecloud_ secret (only present on creation).
// POST /api/tokens → Token
func (c *Client) CreateToken(ctx context.Context, req CreateTokenRequest) (*Token, error) {
	var out Token
	if err := c.doJSON(ctx, "POST", c.serviceURL(authSvc), "/api/tokens", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateServiceAccountToken creates a token bound to a service account. The
// token carries no embedded scopes — it inherits the service account's scopes.
// POST /api/service-accounts/{id}/tokens → Token (Token.Token holds the raw
// ecloud_ secret, only present on creation).
func (c *Client) CreateServiceAccountToken(ctx context.Context, saID, name, expiresIn string) (*Token, error) {
	var out Token
	body := struct {
		Name      string `json:"name"`
		ExpiresIn string `json:"expires_in"`
	}{Name: name, ExpiresIn: expiresIn}
	path := "/api/service-accounts/" + saID + "/tokens"
	if err := c.doJSON(ctx, "POST", c.serviceURL(authSvc), path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteToken deletes (revokes) an API token by ID — works for both standalone
// and service-account-bound tokens (they share the api_tokens table).
// DELETE /api/tokens/{id} → {"status":"ok"}
func (c *Client) DeleteToken(ctx context.Context, id string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(authSvc), "/api/tokens/"+id, nil, nil)
}
