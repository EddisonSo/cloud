package eddsdk

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

const computeSvc = "compute"

func (c *Client) ListContainers(ctx context.Context) ([]Container, error) {
	var out struct {
		Containers []Container `json:"containers"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(computeSvc), "/compute/containers", nil, &out); err != nil {
		return nil, err
	}
	return out.Containers, nil
}

func (c *Client) GetContainer(ctx context.Context, id string) (*Container, error) {
	var out Container
	if err := c.doJSON(ctx, "GET", c.serviceURL(computeSvc), "/compute/containers/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateContainer(ctx context.Context, req CreateContainerRequest) (*Container, error) {
	var out Container
	if err := c.doJSON(ctx, "POST", c.serviceURL(computeSvc), "/compute/containers", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) StartContainer(ctx context.Context, id string) error {
	return c.doJSON(ctx, "POST", c.serviceURL(computeSvc), "/compute/containers/"+id+"/start", nil, nil)
}

func (c *Client) StopContainer(ctx context.Context, id string) error {
	return c.doJSON(ctx, "POST", c.serviceURL(computeSvc), "/compute/containers/"+id+"/stop", nil, nil)
}

func (c *Client) DeleteContainer(ctx context.Context, id string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(computeSvc), "/compute/containers/"+id, nil, nil)
}

func (c *Client) SetPullPolicy(ctx context.Context, id, policy string) error {
	return c.doJSON(ctx, "PUT", c.serviceURL(computeSvc), "/compute/containers/"+id+"/pull-policy",
		map[string]string{"pull_policy": policy}, nil)
}

func (c *Client) SetSSH(ctx context.Context, id string, enabled bool) error {
	return c.doJSON(ctx, "PUT", c.serviceURL(computeSvc), "/compute/containers/"+id+"/ssh",
		map[string]bool{"ssh_enabled": enabled}, nil)
}

// ListIngress returns ingress rules for a container.
// The API responds with {"rules": [{id, port, target_port, created_at}]}.
func (c *Client) ListIngress(ctx context.Context, id string) ([]IngressRule, error) {
	var out struct {
		Rules []IngressRule `json:"rules"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(computeSvc), "/compute/containers/"+id+"/ingress", nil, &out); err != nil {
		return nil, err
	}
	return out.Rules, nil
}

func (c *Client) AddIngress(ctx context.Context, id string, port, target int) error {
	return c.doJSON(ctx, "POST", c.serviceURL(computeSvc), "/compute/containers/"+id+"/ingress",
		map[string]int{"port": port, "target_port": target}, nil)
}

func (c *Client) RemoveIngress(ctx context.Context, id string, port int) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(computeSvc),
		fmt.Sprintf("/compute/containers/%s/ingress/%d", id, port), nil, nil)
}

// GetMounts returns mount paths for a container.
// The API responds with {"mount_paths": [...]}.
func (c *Client) GetMounts(ctx context.Context, id string) ([]string, error) {
	var out struct {
		MountPaths []string `json:"mount_paths"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(computeSvc), "/compute/containers/"+id+"/mounts", nil, &out); err != nil {
		return nil, err
	}
	return out.MountPaths, nil
}

func (c *Client) SetMounts(ctx context.Context, id string, paths []string) error {
	return c.doJSON(ctx, "PUT", c.serviceURL(computeSvc), "/compute/containers/"+id+"/mounts",
		map[string][]string{"mount_paths": paths}, nil)
}

// ListSSHKeys returns all SSH keys for the authenticated user.
// The API responds with {"ssh_keys": [{id, name, public_key, created_at}]}.
func (c *Client) ListSSHKeys(ctx context.Context) ([]SSHKey, error) {
	var out struct {
		SSHKeys []SSHKey `json:"ssh_keys"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(computeSvc), "/compute/ssh-keys", nil, &out); err != nil {
		return nil, err
	}
	return out.SSHKeys, nil
}

// AddSSHKey creates a new SSH key for the authenticated user.
// POST /compute/ssh-keys with body {"name": name, "public_key": publicKey}.
func (c *Client) AddSSHKey(ctx context.Context, name, publicKey string) (*SSHKey, error) {
	var out SSHKey
	body := map[string]string{"name": name, "public_key": publicKey}
	if err := c.doJSON(ctx, "POST", c.serviceURL(computeSvc), "/compute/ssh-keys", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteSSHKey deletes an SSH key by its string ID (decimal integer).
// DELETE /compute/ssh-keys/{id}
func (c *Client) DeleteSSHKey(ctx context.Context, id string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(computeSvc), "/compute/ssh-keys/"+id, nil, nil)
}

// ContainerLogs fetches raw log text from the container logs endpoint.
func (c *Client) ContainerLogs(ctx context.Context, id string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		c.serviceURL(computeSvc)+"/compute/containers/"+id+"/logs", nil)
	if err != nil {
		return "", err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &APIError{Status: resp.StatusCode, Message: string(data)}
	}
	return string(data), nil
}
