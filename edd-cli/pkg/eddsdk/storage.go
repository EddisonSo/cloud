package eddsdk

import "context"

const storageSvc = "storage"

// ListNamespaces returns all visible namespaces from the storage service.
// GET /storage/namespaces → []Namespace
func (c *Client) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	var out []Namespace
	if err := c.doJSON(ctx, "GET", c.serviceURL(storageSvc), "/storage/namespaces", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateNamespace creates a new namespace.
// POST /storage/namespaces → Namespace
func (c *Client) CreateNamespace(ctx context.Context, name string) (*Namespace, error) {
	var out Namespace
	if err := c.doJSON(ctx, "POST", c.serviceURL(storageSvc), "/storage/namespaces",
		CreateNamespaceRequest{Name: name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteNamespace deletes a namespace and all its files.
// DELETE /storage/namespaces/{name} → {"status":"ok"}
func (c *Client) DeleteNamespace(ctx context.Context, name string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(storageSvc), "/storage/namespaces/"+name, nil, nil)
}

// ListFiles lists files in a namespace.
// GET /storage/files?namespace=<ns> → []FileInfo
func (c *Client) ListFiles(ctx context.Context, ns string) ([]FileInfo, error) {
	var out []FileInfo
	path := "/storage/files"
	if ns != "" {
		path += "?namespace=" + ns
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(storageSvc), path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteFile deletes a file from a namespace.
// DELETE /storage/{namespace}/{file} → {"status":"ok","name":<name>}
func (c *Client) DeleteFile(ctx context.Context, ns, path string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(storageSvc),
		"/storage/"+ns+"/"+path, nil, nil)
}
