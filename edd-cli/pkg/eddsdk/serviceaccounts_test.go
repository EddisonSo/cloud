package eddsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListServiceAccounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/service-accounts" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "sa1", "name": "deploy-bot", "scopes": map[string]any{"compute.u1.containers": []string{"read"}}, "token_count": 2, "created_at": 1700000000},
		})
	}))
	defer srv.Close()
	sas, err := newTestClient(srv).ListServiceAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sas) != 1 || sas[0].ID != "sa1" || sas[0].Name != "deploy-bot" {
		t.Fatalf("got %+v", sas)
	}
	if sas[0].TokenCount != 2 {
		t.Fatalf("expected token_count=2 got %d", sas[0].TokenCount)
	}
}

func TestCreateServiceAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/service-accounts" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "ci-bot" {
			t.Errorf("expected name=ci-bot got %v", body["name"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": "sa2", "name": "ci-bot", "scopes": body["scopes"], "token_count": 0, "created_at": 1700000000,
		})
	}))
	defer srv.Close()
	scopes := map[string][]string{"compute.u1.containers": {"read", "start"}}
	sa, err := newTestClient(srv).CreateServiceAccount(context.Background(), "ci-bot", scopes)
	if err != nil {
		t.Fatal(err)
	}
	if sa.ID != "sa2" || sa.Name != "ci-bot" {
		t.Fatalf("got %+v", sa)
	}
}

func TestDeleteServiceAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/service-accounts/sa1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()
	if err := newTestClient(srv).DeleteServiceAccount(context.Background(), "sa1"); err != nil {
		t.Fatal(err)
	}
}

func TestListTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/tokens" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "tok1", "name": "ci-token", "scopes": map[string]any{}, "expires_at": 0, "last_used_at": 0, "created_at": 1700000000},
		})
	}))
	defer srv.Close()
	toks, err := newTestClient(srv).ListTokens(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(toks) != 1 || toks[0].ID != "tok1" || toks[0].Name != "ci-token" {
		t.Fatalf("got %+v", toks)
	}
}

func TestCreateToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/tokens" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "deploy" {
			t.Errorf("expected name=deploy got %v", body["name"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": "tok2", "name": "deploy", "scopes": body["scopes"],
			"expires_at": 0, "last_used_at": 0, "created_at": 1700000000,
			"token": "ecloud_eyJ...",
		})
	}))
	defer srv.Close()
	scopes := map[string][]string{"compute.u1.containers": {"read"}}
	tok, err := newTestClient(srv).CreateToken(context.Background(), CreateTokenRequest{
		Name: "deploy", Scopes: scopes, ExpiresIn: "30d",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok.ID != "tok2" || tok.Token != "ecloud_eyJ..." {
		t.Fatalf("got %+v", tok)
	}
}
