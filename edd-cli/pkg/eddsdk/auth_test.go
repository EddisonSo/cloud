package eddsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogin2FA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/login" || r.Method != "POST" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"requires_2fa": true, "challenge_token": "ch123"})
	}))
	defer srv.Close()
	c := NewClient(Options{})
	c.urlOverride = srv.URL
	res, err := c.Login(context.Background(), "eddison", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Requires2FA || res.ChallengeToken != "ch123" {
		t.Fatalf("got %+v", res)
	}
}

func TestLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/login" || r.Method != "POST" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"token": "sess123", "user_id": "u1", "requires_2fa": false})
	}))
	defer srv.Close()
	c := NewClient(Options{})
	c.urlOverride = srv.URL
	res, err := c.Login(context.Background(), "eddison", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "sess123" || res.Requires2FA {
		t.Fatalf("got %+v", res)
	}
}
