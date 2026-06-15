package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"eddisonso.com/edd-gateway/internal/auth"
	"github.com/golang-jwt/jwt/v5"
)

func TestHasPermission(t *testing.T) {
	granted := map[string][]string{
		"networking.u1.domains":         {"read", "create"},
		"networking.u1.domain-mappings": {"read"},
	}
	cases := []struct {
		scope, action string
		want          bool
	}{
		{"networking.u1.domains", "read", true},
		{"networking.u1.domains", "create", true},
		{"networking.u1.domains", "delete", false}, // action not granted
		{"networking.u1.domain-mappings", "read", true},
		{"networking.u1.domain-mappings", "create", false}, // action not granted
		{"networking.u1.zones", "read", false},             // resource not granted
	}
	for _, c := range cases {
		if got := hasPermission(granted, c.scope, c.action); got != c.want {
			t.Errorf("hasPermission(%q, %q) = %v, want %v", c.scope, c.action, got, c.want)
		}
	}
}

func TestHasPermission_CascadeFromUserRoot(t *testing.T) {
	// A user-root grant (networking.u1) cascades down to any resource...
	granted := map[string][]string{"networking.u1": {"read"}}
	if !hasPermission(granted, "networking.u1.domains", "read") {
		t.Error("user-root grant should cascade to resource")
	}
	// ...but the bare root (networking) is never assignable / checked.
	if hasPermission(map[string][]string{"networking": {"read"}}, "networking.u1.domains", "read") {
		t.Error("bare root must not grant access")
	}
}

func TestActionForMethod(t *testing.T) {
	cases := map[string]string{
		http.MethodGet:    "read",
		http.MethodDelete: "delete",
		http.MethodPost:   "create",
		http.MethodPut:    "create", // default bucket
	}
	for method, want := range cases {
		r := httptest.NewRequest(method, "/api/domains", nil)
		if got := actionForMethod(r); got != want {
			t.Errorf("actionForMethod(%s) = %q, want %q", method, got, want)
		}
	}
}

const testSecret = "test-secret-for-gateway-rbac-0123456789"

func mintToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	claims["exp"] = time.Now().Add(time.Hour).Unix()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// authScopedResult runs authScoped with the given token+method and reports
// whether the wrapped handler was reached and the HTTP status.
func authScopedResult(t *testing.T, s *Server, resource, method, token string) (reached bool, code int) {
	t.Helper()
	h := s.authScoped(resource, func(w http.ResponseWriter, r *http.Request, userID string) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(method, "/api/"+resource, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h(w, req)
	return reached, w.Code
}

// authScopedAtPath is like authScopedResult but lets the test set the exact
// request path (needed for by-id routes like /api/domains/<id>).
func authScopedAtPath(t *testing.T, s *Server, resource, method, path, token string) (reached bool, code int) {
	t.Helper()
	h := s.authScoped(resource, func(w http.ResponseWriter, r *http.Request, userID string) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h(w, req)
	return reached, w.Code
}

func TestResourceIDFromPath(t *testing.T) {
	cases := []struct{ path, resource, want string }{
		{"/api/domains", "domains", ""},
		{"/api/domains/", "domains", ""},
		{"/api/domains/conn123", "domains", "conn123"},
		{"/api/domains/conn123/refresh", "domains", "conn123"},
		{"/api/domain-mappings/m1/verify", "domain-mappings", "m1"},
		{"/api/domain-mappings", "domain-mappings", ""},
	}
	for _, c := range cases {
		if got := resourceIDFromPath(c.path, c.resource); got != c.want {
			t.Errorf("resourceIDFromPath(%q,%q)=%q want %q", c.path, c.resource, got, c.want)
		}
	}
}

func TestAuthScoped_SpecificResource(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	s := &Server{validator: auth.NewSessionValidator()}

	// Token scoped to ONE specific mapping (m1) with delete.
	saM1 := mintToken(t, jwt.MapClaims{
		"user_id": "u1", "type": "api_token",
		"scopes": map[string][]string{"networking.u1.domain-mappings.m1": {"delete"}},
	})
	// DELETE that exact mapping → allowed.
	if reached, code := authScopedAtPath(t, s, "domain-mappings", http.MethodDelete, "/api/domain-mappings/m1", saM1); !reached || code != http.StatusOK {
		t.Errorf("delete m1: reached=%v code=%d, want true/200", reached, code)
	}
	// DELETE a DIFFERENT mapping → forbidden.
	if reached, code := authScopedAtPath(t, s, "domain-mappings", http.MethodDelete, "/api/domain-mappings/m2", saM1); reached || code != http.StatusForbidden {
		t.Errorf("delete m2: reached=%v code=%d, want false/403", reached, code)
	}
	// LIST (collection, needs resource-level read) → forbidden for a per-id token.
	if reached, code := authScopedAtPath(t, s, "domain-mappings", http.MethodGet, "/api/domain-mappings", saM1); reached || code != http.StatusForbidden {
		t.Errorf("list with per-id token: reached=%v code=%d, want false/403", reached, code)
	}

	// A resource-level grant satisfies a specific-id request via cascade.
	saAll := mintToken(t, jwt.MapClaims{
		"user_id": "u1", "type": "api_token",
		"scopes": map[string][]string{"networking.u1.domain-mappings": {"delete"}},
	})
	if reached, code := authScopedAtPath(t, s, "domain-mappings", http.MethodDelete, "/api/domain-mappings/m1", saAll); !reached || code != http.StatusOK {
		t.Errorf("resource-level delete m1: reached=%v code=%d, want true/200", reached, code)
	}
}

func TestAuthScoped(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	s := &Server{validator: auth.NewSessionValidator()}

	// Session token (no type/scopes) → full access regardless of method.
	session := mintToken(t, jwt.MapClaims{"user_id": "u1"})
	if reached, code := authScopedResult(t, s, "domains", http.MethodGet, session); !reached || code != http.StatusOK {
		t.Errorf("session GET: reached=%v code=%d, want true/200", reached, code)
	}
	if reached, code := authScopedResult(t, s, "domains", http.MethodDelete, session); !reached || code != http.StatusOK {
		t.Errorf("session DELETE: reached=%v code=%d, want true/200", reached, code)
	}

	// SA token with the matching scope+action → allowed.
	saRead := mintToken(t, jwt.MapClaims{
		"user_id": "u1", "type": "api_token",
		"scopes": map[string][]string{"networking.u1.domains": {"read"}},
	})
	if reached, code := authScopedResult(t, s, "domains", http.MethodGet, saRead); !reached || code != http.StatusOK {
		t.Errorf("SA read GET: reached=%v code=%d, want true/200", reached, code)
	}
	// Same token, DELETE (needs delete) → forbidden.
	if reached, code := authScopedResult(t, s, "domains", http.MethodDelete, saRead); reached || code != http.StatusForbidden {
		t.Errorf("SA read DELETE: reached=%v code=%d, want false/403", reached, code)
	}

	// SA token with no networking scope at all → forbidden.
	saOther := mintToken(t, jwt.MapClaims{
		"user_id": "u1", "type": "api_token",
		"scopes": map[string][]string{"compute.u1.containers": {"read"}},
	})
	if reached, code := authScopedResult(t, s, "domains", http.MethodGet, saOther); reached || code != http.StatusForbidden {
		t.Errorf("SA no-networking GET: reached=%v code=%d, want false/403", reached, code)
	}

	// SA token scoped to domains must not reach domain-mappings.
	if reached, code := authScopedResult(t, s, "domain-mappings", http.MethodGet, saRead); reached || code != http.StatusForbidden {
		t.Errorf("SA domains scope on domain-mappings: reached=%v code=%d, want false/403", reached, code)
	}
}

func TestAuthScoped_RejectsNonSessionTokenTypes(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	s := &Server{validator: auth.NewSessionValidator()}
	// Validly-signed tokens with these types must NOT be honoured as sessions.
	for _, typ := range []string{"2fa_challenge", "repository"} {
		token := mintToken(t, jwt.MapClaims{"user_id": "u1", "type": typ})
		reached, code := authScopedResult(t, s, "domains", http.MethodGet, token)
		if reached || code != http.StatusUnauthorized {
			t.Errorf("type %q: reached=%v code=%d, want false/401", typ, reached, code)
		}
	}
}

func TestAuthScoped_RejectsRegistryToken(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	s := &Server{validator: auth.NewSessionValidator()}
	// Registry tokens carry type="" (same as interactive sessions) but no user_id.
	// The UserID guard in authenticate() must reject them before they reach a handler.
	token := mintToken(t, jwt.MapClaims{
		// No "user_id" claim — registry tokens don't have one.
		"sub": "registry-subject",
	})
	reached, code := authScopedResult(t, s, "domains", http.MethodGet, token)
	if reached || code != http.StatusUnauthorized {
		t.Errorf("registry token: reached=%v code=%d, want false/401", reached, code)
	}
}

func TestAuthScoped_Unauthenticated(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	s := &Server{validator: auth.NewSessionValidator()}
	h := s.authScoped("domains", func(w http.ResponseWriter, r *http.Request, userID string) {
		t.Error("handler must not be reached without a token")
	})
	req := httptest.NewRequest(http.MethodGet, "/api/domains", nil)
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}
