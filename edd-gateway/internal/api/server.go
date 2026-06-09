// Package api serves the authenticated custom-domains management API. It runs on
// loopback; the gateway exposes it via a static route at net.cloud.eddisonso.com.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"eddisonso.com/edd-gateway/internal/auth"
	"eddisonso.com/edd-gateway/internal/cloudflare"
	"eddisonso.com/edd-gateway/internal/domains"
	"eddisonso.com/edd-gateway/internal/router"
	"eddisonso.com/edd-gateway/internal/secretbox"
)

// ingressTarget is the stable host custom-domain CNAMEs point at (covered by
// the DDNS-maintained *.cloud.eddisonso.com wildcard).
const ingressTarget = "ingress.cloud.eddisonso.com"

// idGen returns a unique id for a new domain row.
type idGen func() string

// Server is the management API.
type Server struct {
	router    *router.Router
	validator *auth.SessionValidator
	newID     idGen
	preIssue  func(domain string)
	box       *secretbox.Box // nil = Cloudflare token integration disabled
}

// New builds the API server.
func New(r *router.Router, v *auth.SessionValidator, newID idGen, preIssue func(string), box *secretbox.Box) *Server {
	return &Server{router: r, validator: v, newID: newID, preIssue: preIssue, box: box}
}

// newCFClient builds a Cloudflare client from a plaintext token; overridable in tests.
var newCFClient = func(token string) *cloudflare.Client { return cloudflare.New(token) }

// userCF returns a Cloudflare client for the user's stored token, or nil if
// the integration is disabled, no token is stored, or decryption fails.
func (s *Server) userCF(userID string) *cloudflare.Client {
	if s.box == nil {
		return nil
	}
	ct, err := s.router.GetCloudflareToken(userID)
	if err != nil {
		return nil // ErrNotFound or DB error -> manual flow
	}
	tok, err := s.box.Open(ct)
	if err != nil {
		slog.Warn("cloudflare token decryption failed (key rotated?)", "user", userID)
		return nil
	}
	return newCFClient(string(tok))
}

// Handler returns the HTTP mux for the API, wrapped in CORS so the dashboard
// at cloud.eddisonso.com can call it cross-origin.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/domains", s.auth(s.handleDomains))
	mux.HandleFunc("/api/domains/", s.auth(s.handleDomainByID))
	mux.HandleFunc("/api/cloudflare-token", s.auth(s.handleCloudflareToken))
	return corsMiddleware(mux)
}

// isAllowedOrigin / corsMiddleware mirror the CORS policy used by the other
// edd-cloud services. The OPTIONS preflight is answered here, before the auth
// middleware — otherwise the browser's tokenless preflight gets a 401 with no
// CORS headers and the real request is blocked as a network error.
func isAllowedOrigin(origin string) bool {
	return origin == "https://cloud.eddisonso.com" ||
		(len(origin) > len("https://.cloud.eddisonso.com") &&
			strings.HasSuffix(origin, ".cloud.eddisonso.com") &&
			strings.HasPrefix(origin, "https://"))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// auth wraps a handler, validating the JWT and passing the user id through.
func (s *Server) auth(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cookie string
		if c, err := r.Cookie("token"); err == nil {
			cookie = c.Value
		}
		tok := auth.ExtractToken(r.Header.Get("Authorization"), cookie)
		if tok == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, err := s.validator.ValidateSession(tok)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r, claims.UserID)
	}
}

type domainResponse struct {
	ID           string `json:"id"`
	Domain       string `json:"domain"`
	ContainerID  string `json:"container_id"`
	TargetPort   int    `json:"target_port"`
	Status       string `json:"status"`
	VerifyName   string `json:"verify_name"`
	VerifyToken  string `json:"verify_token"`
	DNSAutomated bool   `json:"dns_automated,omitempty"`
}

func toResponse(cd *router.CustomDomain) domainResponse {
	return domainResponse{
		ID: cd.ID, Domain: cd.Domain, ContainerID: cd.ContainerID,
		TargetPort: cd.TargetPort, Status: cd.Status,
		VerifyName: domains.VerifyRecordName(cd.Domain), VerifyToken: cd.VerifyToken,
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// handleDomains: GET list, POST create.
func (s *Server) handleDomains(w http.ResponseWriter, r *http.Request, userID string) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.router.ListCustomDomainsByUser(userID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		out := make([]domainResponse, 0, len(list))
		for _, cd := range list {
			out = append(out, toResponse(cd))
		}
		writeJSON(w, http.StatusOK, map[string]any{"domains": out})
	case http.MethodPost:
		s.createDomain(w, r, userID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type createRequest struct {
	ContainerID string `json:"container_id"`
	Domain      string `json:"domain"`
	TargetPort  int    `json:"target_port"`
}

// allowedPort mirrors the compute ingress rules: 80, 443, or 8000-8999.
func allowedPort(p int) bool {
	return p == 80 || p == 443 || (p >= 8000 && p <= 8999)
}

func (s *Server) createDomain(w http.ResponseWriter, r *http.Request, userID string) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	d := domains.Normalize(req.Domain)
	if !domains.Valid(d) {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}
	if !allowedPort(req.TargetPort) {
		http.Error(w, "port must be 80, 443, or 8000-8999", http.StatusBadRequest)
		return
	}
	owner, err := s.router.ContainerOwner(req.ContainerID)
	if err != nil {
		http.Error(w, "container not found", http.StatusNotFound)
		return
	}
	if owner != userID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	cd := &router.CustomDomain{
		ID: s.newID(), UserID: userID, ContainerID: req.ContainerID,
		Domain: d, TargetPort: req.TargetPort,
		VerifyToken: domains.GenerateToken(), Status: "pending",
	}
	if err := s.router.CreateCustomDomain(cd); err != nil {
		if errors.Is(err, router.ErrDomainExists) {
			http.Error(w, "domain already in use", http.StatusConflict)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// DNS automation runs only AFTER the row exists: the upsert deliberately
	// overwrites whatever record sits at this name, so it must never fire for
	// a create that fails (e.g. duplicate-domain 409) — that would rewrite the
	// user's zone while reporting an error.
	dnsAutomated := false
	if cf := s.userCF(userID); cf != nil {
		zoneID, err := cf.FindZone(d)
		switch {
		case err == nil:
			if err := cf.UpsertCNAME(zoneID, d, ingressTarget); err == nil {
				if err := s.router.SetCustomDomainStatus(cd.ID, "verified", true); err == nil {
					cd.Status = "verified"
					dnsAutomated = true
					if s.preIssue != nil {
						s.preIssue(d)
					}
				} else {
					slog.Error("failed to mark domain verified after DNS automation", "domain", d, "error", err)
				}
			} else {
				slog.Warn("cloudflare CNAME upsert failed; falling back to manual verification", "domain", d, "error", err)
			}
		case !errors.Is(err, cloudflare.ErrZoneNotFound):
			slog.Warn("cloudflare zone lookup failed; falling back to manual verification", "domain", d, "error", err)
		}
	}
	slog.Info("custom domain created", "domain", d, "user", userID, "container", req.ContainerID, "dns_automated", dnsAutomated)
	resp := toResponse(cd)
	resp.DNSAutomated = dnsAutomated
	writeJSON(w, http.StatusCreated, resp)
}

// handleDomainByID: DELETE /api/domains/{id}, POST /api/domains/{id}/verify.
func (s *Server) handleDomainByID(w http.ResponseWriter, r *http.Request, userID string) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/domains/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(rest, "/verify") && r.Method == http.MethodPost {
		id := strings.TrimSuffix(rest, "/verify")
		s.verifyNow(w, r, userID, id)
		return
	}
	if r.Method == http.MethodDelete {
		id := rest
		cd, err := s.router.GetCustomDomain(id)
		if err != nil || cd.UserID != userID {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := s.router.DeleteCustomDomain(id, userID); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if cf := s.userCF(userID); cf != nil {
			zoneID, err := cf.FindZone(cd.Domain)
			switch {
			case err == nil:
				if err := cf.DeleteRecord(zoneID, cd.Domain, ingressTarget); err != nil {
					slog.Warn("cloudflare record cleanup failed", "domain", cd.Domain, "error", err)
				}
			case !errors.Is(err, cloudflare.ErrZoneNotFound):
				slog.Warn("cloudflare record cleanup skipped: zone lookup failed", "domain", cd.Domain, "error", err)
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// verifyNow runs an immediate DNS TXT check for one domain.
func (s *Server) verifyNow(w http.ResponseWriter, r *http.Request, userID, id string) {
	cd, err := s.router.GetCustomDomain(id)
	if err != nil || cd.UserID != userID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	records, _ := lookupTXT(domains.VerifyRecordName(cd.Domain))
	if domains.TXTMatches(records, cd.VerifyToken) {
		if err := s.router.SetCustomDomainStatus(cd.ID, "verified", true); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if s.preIssue != nil {
			s.preIssue(cd.Domain)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
		return
	}
	// No match. If the domain had expired to 'failed', reset it to 'pending' so
	// the background worker resumes polling and the 7-day window restarts — this
	// is the documented retry path.
	if cd.Status == "failed" {
		if err := s.router.SetCustomDomainStatus(cd.ID, "pending", false); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "pending",
		"detail": fmt.Sprintf("TXT %s not found or does not match", domains.VerifyRecordName(cd.Domain)),
	})
}
