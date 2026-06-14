package eddsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(srv *httptest.Server) *Client {
	c := NewClient(Options{Token: "tok"})
	c.urlOverride = srv.URL
	return c
}

func TestListContainers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/compute/containers" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"containers": []map[string]any{
			{"id": "abc", "name": "web", "status": "running", "pull_policy": "Always"},
		}})
	}))
	defer srv.Close()
	cs, err := newTestClient(srv).ListContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].ID != "abc" || cs[0].PullPolicy != "Always" {
		t.Fatalf("got %+v", cs)
	}
}

func TestStartStopPullPolicy(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.Method+" "+r.URL.Path)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c := newTestClient(srv)
	ctx := context.Background()
	if err := c.StartContainer(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	if err := c.StopContainer(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetPullPolicy(ctx, "abc", "Always"); err != nil {
		t.Fatal(err)
	}
	want := []string{"POST /compute/containers/abc/start", "POST /compute/containers/abc/stop", "PUT /compute/containers/abc/pull-policy"}
	for i, w := range want {
		if i >= len(hits) || hits[i] != w {
			t.Fatalf("hits=%v want %v", hits, want)
		}
	}
}

func TestListSSHKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/compute/ssh-keys" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"ssh_keys": []map[string]any{
			{"id": 1, "name": "my-key", "public_key": "ssh-ed25519 AAAA...", "created_at": "2026-01-01T00:00:00Z"},
		}})
	}))
	defer srv.Close()
	keys, err := newTestClient(srv).ListSSHKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Name != "my-key" || keys[0].ID != 1 {
		t.Fatalf("got %+v", keys)
	}
}

func TestContainerLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/compute/containers/abc/logs" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(200)
		w.Write([]byte("line1\nline2\n"))
	}))
	defer srv.Close()
	logs, err := newTestClient(srv).ContainerLogs(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if logs != "line1\nline2\n" {
		t.Fatalf("got %q", logs)
	}
}
