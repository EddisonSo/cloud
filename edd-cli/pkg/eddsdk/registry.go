package eddsdk

import "context"

const registrySvc = "registry"

// ListRepos returns all repositories visible to the caller.
// GET /api/repos → {"repositories": [...]}
func (c *Client) ListRepos(ctx context.Context) ([]Repo, error) {
	var out struct {
		Repositories []Repo `json:"repositories"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(registrySvc), "/api/repos", nil, &out); err != nil {
		return nil, err
	}
	return out.Repositories, nil
}

// ListTags returns all tags for a repository.
// GET /api/repos/{name}/tags → {"name": ..., "tags": [...]}
func (c *Client) ListTags(ctx context.Context, repo string) ([]Tag, error) {
	var out struct {
		Tags []Tag `json:"tags"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(registrySvc), "/api/repos/"+repo+"/tags", nil, &out); err != nil {
		return nil, err
	}
	return out.Tags, nil
}

// DeleteTag deletes a tag from a repository.
// DELETE /api/repos/{name}/tags/{tag} → 204
func (c *Client) DeleteTag(ctx context.Context, repo, ref string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(registrySvc),
		"/api/repos/"+repo+"/tags/"+ref, nil, nil)
}
