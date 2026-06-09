package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"eddisonso.com/edd-gateway/internal/cloudflare"
)

// handleCloudflareToken serves PUT/GET/DELETE /api/cloudflare-token.
func (s *Server) handleCloudflareToken(w http.ResponseWriter, r *http.Request, userID string) {
	if s.box == nil {
		http.Error(w, "cloudflare integration not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
			http.Error(w, "token is required", http.StatusBadRequest)
			return
		}
		// Validate the token by listing zones with it before storing.
		zones, err := newCFClient(req.Token).ListZones()
		if err != nil {
			http.Error(w, "token invalid or lacks zone access", http.StatusBadRequest)
			return
		}
		if err := s.router.SetCloudflareToken(userID, s.box.Seal([]byte(req.Token))); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		slog.Info("cloudflare token connected", "user", userID, "zones", len(zones))
		writeJSON(w, http.StatusOK, map[string]any{"configured": true, "zones": zoneNames(zones)})
	case http.MethodGet:
		cf := s.userCF(userID)
		if cf == nil {
			writeJSON(w, http.StatusOK, map[string]any{"configured": false})
			return
		}
		zones, err := cf.ListZones()
		if err != nil {
			// Token stored but no longer working (revoked?). Report configured
			// with no zones so the UI can suggest reconnecting.
			writeJSON(w, http.StatusOK, map[string]any{"configured": true, "zones": []string{}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"configured": true, "zones": zoneNames(zones)})
	case http.MethodDelete:
		if err := s.router.DeleteCloudflareToken(userID); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func zoneNames(zs []cloudflare.Zone) []string {
	names := make([]string, 0, len(zs))
	for _, z := range zs {
		names = append(names, z.Name)
	}
	return names
}
