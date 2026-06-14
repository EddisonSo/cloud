package eddsdk

import "context"

const authSvc = "auth"

// LoginResult is the response from POST /api/login.
type LoginResult struct {
	Token       string `json:"token"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      string `json:"user_id"`
	IsAdmin     bool   `json:"is_admin"`
	Requires2FA bool   `json:"requires_2fa"`
}

// Session is the response from GET /api/session.
// Fields match edd-cloud-auth sessionResponse: username, display_name, user_id, is_admin, token.
type Session struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      string `json:"user_id"`
	IsAdmin     bool   `json:"is_admin"`
	Token       string `json:"token,omitempty"`
}

func (c *Client) Login(ctx context.Context, username, password string) (*LoginResult, error) {
	var out LoginResult
	body := map[string]string{"username": username, "password": password}
	if err := c.doJSON(ctx, "POST", c.serviceURL(authSvc), "/api/login", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Session(ctx context.Context) (*Session, error) {
	var out Session
	if err := c.doJSON(ctx, "GET", c.serviceURL(authSvc), "/api/session", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
