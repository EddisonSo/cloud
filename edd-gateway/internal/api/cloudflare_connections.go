// This file serves /api/domains (the user's owned domains, backed by per-zone
// Cloudflare tokens). Despite the filename, these handlers back the
// /api/domains route group; the hostname->container routes live in server.go
// under /api/domain-mappings.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"eddisonso.com/edd-gateway/internal/cloudflare"
	"eddisonso.com/edd-gateway/internal/router"
)

type connectionResponse struct {
	ID        string   `json:"id"`
	Zones     []string `json:"zones"`
	CreatedAt string   `json:"created_at"`
}

func toConnectionResponse(c *router.CloudflareConnection) connectionResponse {
	zones := c.Zones
	if zones == nil {
		zones = []string{}
	}
	return connectionResponse{ID: c.ID, Zones: zones, CreatedAt: c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")}
}

// handleDomains serves GET (list) and POST (add) /api/domains — the user's
// owned domains, each backed by a per-zone Cloudflare token.
func (s *Server) handleDomains(w http.ResponseWriter, r *http.Request, userID string) {
	if s.box == nil {
		http.Error(w, "cloudflare integration not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		conns, err := s.router.ListCloudflareConnections(userID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		out := make([]connectionResponse, 0, len(conns))
		for i := range conns {
			// Lazily backfill zones for rows migrated from the single-token era.
			if len(conns[i].Zones) == 0 {
				if tok, err := s.box.Open(conns[i].Ciphertext); err == nil {
					if zones, err := newCFClient(string(tok)).ListZones(); err == nil {
						names := zoneNames(zones)
						if err := s.router.UpdateCloudflareConnectionZones(conns[i].ID, userID, names); err == nil {
							conns[i].Zones = names
						}
					}
				}
			}
			out = append(out, toConnectionResponse(&conns[i]))
		}
		writeJSON(w, http.StatusOK, map[string]any{"connections": out})
	case http.MethodPost:
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
			http.Error(w, "token is required", http.StatusBadRequest)
			return
		}
		zones, err := newCFClient(req.Token).ListZones()
		if err != nil {
			http.Error(w, "token invalid or lacks zone access", http.StatusBadRequest)
			return
		}
		names := zoneNames(zones)
		id, err := s.router.AddCloudflareConnection(userID, s.box.Seal([]byte(req.Token)), names)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		slog.Info("cloudflare connection added", "user", userID, "connection", id, "zones", len(names))
		writeJSON(w, http.StatusCreated, connectionResponse{ID: id, Zones: names})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDomainByID serves DELETE /api/domains/{id}
// and POST /api/domains/{id}/refresh.
func (s *Server) handleDomainByID(w http.ResponseWriter, r *http.Request, userID string) {
	if s.box == nil {
		http.Error(w, "cloudflare integration not configured", http.StatusServiceUnavailable)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/domains/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(rest, "/refresh") && r.Method == http.MethodPost {
		s.refreshConnection(w, userID, strings.TrimSuffix(rest, "/refresh"))
		return
	}
	if r.Method == http.MethodDelete {
		if err := s.router.DeleteCloudflareConnection(rest, userID); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// refreshConnection re-snapshots a connection's zones with its stored token.
func (s *Server) refreshConnection(w http.ResponseWriter, userID, id string) {
	conns, err := s.router.ListCloudflareConnections(userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var conn *router.CloudflareConnection
	for i := range conns {
		if conns[i].ID == id {
			conn = &conns[i]
		}
	}
	if conn == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	tok, err := s.box.Open(conn.Ciphertext)
	if err != nil {
		http.Error(w, "token unreadable — disconnect and reconnect", http.StatusConflict)
		return
	}
	zones, err := newCFClient(string(tok)).ListZones()
	if err != nil {
		http.Error(w, "token no longer valid — disconnect and reconnect", http.StatusBadGateway)
		return
	}
	names := zoneNames(zones)
	if err := s.router.UpdateCloudflareConnectionZones(id, userID, names); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, connectionResponse{ID: id, Zones: names})
}

func zoneNames(zs []cloudflare.Zone) []string {
	names := make([]string, 0, len(zs))
	for _, z := range zs {
		names = append(names, z.Name)
	}
	return names
}
