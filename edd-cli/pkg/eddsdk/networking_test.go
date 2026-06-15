package eddsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListDomains(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/domains" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"connections": []map[string]any{
				{"id": "cf1", "zones": []string{"example.com"}, "created_at": "2026-01-01T00:00:00Z"},
			},
		})
	}))
	defer srv.Close()
	domains, err := newTestClient(srv).ListDomains(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0].ID != "cf1" || len(domains[0].Zones) != 1 {
		t.Fatalf("got %+v", domains)
	}
}

func TestAddDomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/domains" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["token"] != "cf-api-token" {
			t.Errorf("expected token=cf-api-token got %v", body["token"])
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id": "cf2", "zones": []string{"example.com", "other.com"}, "created_at": "2026-01-02T00:00:00Z",
		})
	}))
	defer srv.Close()
	d, err := newTestClient(srv).AddDomain(context.Background(), "cf-api-token")
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "cf2" || len(d.Zones) != 2 {
		t.Fatalf("got %+v", d)
	}
}

func TestDeleteDomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/domains/cf1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if err := newTestClient(srv).DeleteDomain(context.Background(), "cf1"); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshDomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/domains/cf1/refresh" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": "cf1", "zones": []string{"example.com"}, "created_at": "2026-01-01T00:00:00Z",
		})
	}))
	defer srv.Close()
	d, err := newTestClient(srv).RefreshDomain(context.Background(), "cf1")
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "cf1" || len(d.Zones) != 1 {
		t.Fatalf("got %+v", d)
	}
}

func TestListDomainMappings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/domain-mappings" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"domains": []map[string]any{
				{"id": "d1", "domain": "app.example.com", "container_id": "c1", "target_port": 8080, "status": "pending", "verify_name": "_verify.app.example.com", "verify_token": "tok123"},
			},
		})
	}))
	defer srv.Close()
	mappings, err := newTestClient(srv).ListDomainMappings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(mappings) != 1 || mappings[0].ID != "d1" || mappings[0].Domain != "app.example.com" {
		t.Fatalf("got %+v", mappings)
	}
}

func TestAddDomainMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/domain-mappings" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["domain"] != "app.example.com" {
			t.Errorf("expected domain=app.example.com got %v", body["domain"])
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id": "d2", "domain": "app.example.com", "container_id": "c1", "target_port": 8080, "status": "pending", "verify_name": "_v.app.example.com", "verify_token": "tok456",
		})
	}))
	defer srv.Close()
	m, err := newTestClient(srv).AddDomainMapping(context.Background(), CreateDomainMappingRequest{
		ContainerID: "c1", Domain: "app.example.com", TargetPort: 8080,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "d2" || m.Status != "pending" {
		t.Fatalf("got %+v", m)
	}
}

func TestDeleteDomainMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/domain-mappings/d1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if err := newTestClient(srv).DeleteDomainMapping(context.Background(), "d1"); err != nil {
		t.Fatal(err)
	}
}
