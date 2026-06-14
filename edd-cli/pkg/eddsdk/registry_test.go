package eddsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListRepos(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/repos" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"repositories": []map[string]any{
				{"name": "myapp", "visibility": 2, "owner_id": "u1", "tag_count": 3, "total_size": 1024, "last_pushed": now},
			},
		})
	}))
	defer srv.Close()
	repos, err := newTestClient(srv).ListRepos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "myapp" || repos[0].TagCount != 3 {
		t.Fatalf("got %+v", repos)
	}
}

func TestListTags(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/repos/myapp/tags" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"name": "myapp",
			"tags": []map[string]any{
				{"name": "latest", "digest": "sha256:abc", "size": 512, "pushed_at": now},
			},
		})
	}))
	defer srv.Close()
	tags, err := newTestClient(srv).ListTags(context.Background(), "myapp")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].Name != "latest" || tags[0].Digest != "sha256:abc" {
		t.Fatalf("got %+v", tags)
	}
}

func TestDeleteTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/repos/myapp/tags/latest" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if err := newTestClient(srv).DeleteTag(context.Background(), "myapp", "latest"); err != nil {
		t.Fatal(err)
	}
}
