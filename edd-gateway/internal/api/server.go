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
	"eddisonso.com/edd-gateway/internal/domains"
	"eddisonso.com/edd-gateway/internal/router"
)

// idGen returns a unique id for a new domain row.
type idGen func() string

// Server is the management API.
type Server struct {
	router    *router.Router
	validator *auth.SessionValidator
	newID     idGen
	preIssue  func(domain string)
}

// New builds the API server.
func New(r *router.Router, v *auth.SessionValidator, newID idGen, preIssue func(string)) *Server {
	return &Server{router: r, validator: v, newID: newID, preIssue: preIssue}
}

// Handler returns the HTTP mux for the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/domains", s.auth(s.handleDomains))
	mux.HandleFunc("/api/domains/", s.auth(s.handleDomainByID))
	return mux
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
	ID          string `json:"id"`
	Domain      string `json:"domain"`
	ContainerID string `json:"container_id"`
	TargetPort  int    `json:"target_port"`
	Status      string `json:"status"`
	VerifyName  string `json:"verify_name"`
	VerifyToken string `json:"verify_token"`
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
	slog.Info("custom domain created", "domain", d, "user", userID, "container", req.ContainerID)
	writeJSON(w, http.StatusCreated, toResponse(cd))
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
