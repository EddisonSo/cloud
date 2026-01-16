package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"eddisonso.com/edd-cloud/services/compute/internal/db"
)

type ingressRuleResponse struct {
	ID        int64  `json:"id"`
	Port      int    `json:"port"`
	Protocol  string `json:"protocol"`
	CreatedAt int64  `json:"created_at"`
}

type ingressResponse struct {
	Rules        []ingressRuleResponse `json:"rules"`
	AllowedPorts []int                 `json:"allowed_ports"`
}

func (h *Handler) ListIngressRules(w http.ResponseWriter, r *http.Request) {
	userID, _, _ := getUserFromContext(r.Context())
	containerID := r.PathValue("id")

	// Verify container ownership
	container, err := h.db.GetContainer(containerID)
	if err != nil || container == nil {
		writeError(w, "container not found", http.StatusNotFound)
		return
	}
	if container.UserID != userID {
		writeError(w, "forbidden", http.StatusForbidden)
		return
	}

	rules, err := h.db.ListIngressRules(containerID)
	if err != nil {
		slog.Error("failed to list ingress rules", "error", err)
		writeError(w, "failed to list ingress rules", http.StatusInternalServerError)
		return
	}

	resp := ingressResponse{
		Rules:        make([]ingressRuleResponse, 0, len(rules)),
		AllowedPorts: db.AllowedPorts,
	}
	for _, rule := range rules {
		resp.Rules = append(resp.Rules, ingressRuleResponse{
			ID:        rule.ID,
			Port:      rule.Port,
			Protocol:  rule.Protocol,
			CreatedAt: rule.CreatedAt.Unix(),
		})
	}

	writeJSON(w, resp)
}

type addIngressRequest struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

func (h *Handler) AddIngressRule(w http.ResponseWriter, r *http.Request) {
	userID, _, _ := getUserFromContext(r.Context())
	containerID := r.PathValue("id")

	// Verify container ownership
	container, err := h.db.GetContainer(containerID)
	if err != nil || container == nil {
		writeError(w, "container not found", http.StatusNotFound)
		return
	}
	if container.UserID != userID {
		writeError(w, "forbidden", http.StatusForbidden)
		return
	}

	var req addIngressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate port is allowed
	portAllowed := false
	for _, p := range db.AllowedPorts {
		if p == req.Port {
			portAllowed = true
			break
		}
	}
	if !portAllowed {
		writeError(w, "port not allowed", http.StatusBadRequest)
		return
	}

	// Validate protocol
	if req.Protocol != "tcp" && req.Protocol != "udp" {
		req.Protocol = "tcp" // Default to TCP
	}

	rule, err := h.db.AddIngressRule(containerID, req.Port, req.Protocol)
	if err != nil {
		slog.Error("failed to add ingress rule", "error", err)
		writeError(w, "failed to add ingress rule", http.StatusInternalServerError)
		return
	}

	// Update NetworkPolicy in Kubernetes
	if err := h.k8s.UpdateNetworkPolicy(r.Context(), container.Namespace, h.getEnabledPorts(containerID)); err != nil {
		slog.Error("failed to update network policy", "error", err)
		// Don't fail the request, the DB is updated
	}

	writeJSON(w, ingressRuleResponse{
		ID:        rule.ID,
		Port:      rule.Port,
		Protocol:  rule.Protocol,
		CreatedAt: rule.CreatedAt.Unix(),
	})
}

func (h *Handler) RemoveIngressRule(w http.ResponseWriter, r *http.Request) {
	userID, _, _ := getUserFromContext(r.Context())
	containerID := r.PathValue("id")
	portStr := r.PathValue("port")

	port, err := strconv.Atoi(portStr)
	if err != nil {
		writeError(w, "invalid port", http.StatusBadRequest)
		return
	}

	// Verify container ownership
	container, err := h.db.GetContainer(containerID)
	if err != nil || container == nil {
		writeError(w, "container not found", http.StatusNotFound)
		return
	}
	if container.UserID != userID {
		writeError(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := h.db.RemoveIngressRule(containerID, port); err != nil {
		slog.Error("failed to remove ingress rule", "error", err)
		writeError(w, "failed to remove ingress rule", http.StatusInternalServerError)
		return
	}

	// Update NetworkPolicy in Kubernetes
	if err := h.k8s.UpdateNetworkPolicy(r.Context(), container.Namespace, h.getEnabledPorts(containerID)); err != nil {
		slog.Error("failed to update network policy", "error", err)
		// Don't fail the request, the DB is updated
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) getEnabledPorts(containerID string) []int {
	rules, err := h.db.ListIngressRules(containerID)
	if err != nil {
		return nil
	}
	var ports []int
	for _, rule := range rules {
		ports = append(ports, rule.Port)
	}
	return ports
}
