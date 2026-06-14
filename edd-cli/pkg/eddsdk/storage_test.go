package eddsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListNamespaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/storage/namespaces" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"name": "default", "count": 3, "hidden": false, "visibility": 2},
			{"name": "private-ns", "count": 1, "hidden": true, "visibility": 0},
		})
	}))
	defer srv.Close()
	ns, err := newTestClient(srv).ListNamespaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 2 || ns[0].Name != "default" || ns[0].Count != 3 {
		t.Fatalf("got %+v", ns)
	}
	if ns[1].Name != "private-ns" || ns[1].Visibility != 0 {
		t.Fatalf("got %+v", ns[1])
	}
}

func TestCreateNamespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/storage/namespaces" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "myns" {
			t.Errorf("expected name=myns got %v", body["name"])
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"name": "myns", "count": 0, "hidden": false, "visibility": 2,
		})
	}))
	defer srv.Close()
	ns, err := newTestClient(srv).CreateNamespace(context.Background(), "myns")
	if err != nil {
		t.Fatal(err)
	}
	if ns.Name != "myns" || ns.Count != 0 {
		t.Fatalf("got %+v", ns)
	}
}

func TestDeleteNamespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/storage/namespaces/myns" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()
	if err := newTestClient(srv).DeleteNamespace(context.Background(), "myns"); err != nil {
		t.Fatal(err)
	}
}

func TestListFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/storage/files" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("namespace") != "default" {
			t.Errorf("expected namespace=default got %q", r.URL.Query().Get("namespace"))
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"name": "hello.txt", "path": "/sfs/default/hello.txt", "namespace": "default", "size": 128, "created_at": 1700000000, "modified_at": 1700000000},
		})
	}))
	defer srv.Close()
	files, err := newTestClient(srv).ListFiles(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "hello.txt" || files[0].Size != 128 {
		t.Fatalf("got %+v", files)
	}
}

func TestDeleteFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/storage/default/hello.txt" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "name": "hello.txt"})
	}))
	defer srv.Close()
	if err := newTestClient(srv).DeleteFile(context.Background(), "default", "hello.txt"); err != nil {
		t.Fatal(err)
	}
}
