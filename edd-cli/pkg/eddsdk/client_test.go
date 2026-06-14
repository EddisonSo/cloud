package eddsdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceURL(t *testing.T) {
	c := NewClient(Options{BaseDomain: "cloud.eddisonso.com", Token: "t"})
	cases := map[string]string{
		"compute":    "https://compute.cloud.eddisonso.com",
		"storage":    "https://storage.cloud.eddisonso.com",
		"networking": "https://net.cloud.eddisonso.com",
	}
	for svc, want := range cases {
		if got := c.serviceURL(svc); got != want {
			t.Errorf("serviceURL(%q) = %q, want %q", svc, got, want)
		}
	}
}

func TestDoJSON_SendsAuthAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing/wrong auth header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
	}))
	defer srv.Close()
	c := NewClient(Options{Token: "tok"})
	var out struct {
		Hello string `json:"hello"`
	}
	if err := c.doJSON(context.Background(), "GET", srv.URL, "/x", nil, &out); err != nil {
		t.Fatalf("doJSON: %v", err)
	}
	if out.Hello != "world" {
		t.Errorf("decoded %q", out.Hello)
	}
}

func TestDoJSON_MapsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewClient(Options{Token: "tok"})
	err := c.doJSON(context.Background(), "GET", srv.URL, "/x", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.Status != 403 || !strings.Contains(apiErr.Message, "nope") {
		t.Errorf("got status=%d msg=%q", apiErr.Status, apiErr.Message)
	}
}
