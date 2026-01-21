package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"eddisonso.com/edd-cloud/services/compute/internal/db"
)

type ingressRuleResponse struct {
	ID         int64  `json:"id"`
	Port       int    `json:"port"`
	TargetPort int    `json:"target_port"`
	CreatedAt  int64  `json:"created_at"`
}

type ingressResponse struct {
	Rules []ingressRuleResponse `json:"rules"`
}

func (h *Handler) ListIngressRules(w http.ResponseWriter, r *http.Request) {
	_, userID, _, _ := getUserFromContext(r.Context())
	containerID := r.PathValue("id")

	// Verify container ownership
	container, err := h.db.GetContainer(containerID)
	if err != nil || container == nil {
		writeError(w, "container not found", http.StatusNotFound)
		return
	}
	if container.OwnerPublicID != userID {
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
		Rules: make([]ingressRuleResponse, 0, len(rules)),
	}
	for _, rule := range rules {
		resp.Rules = append(resp.Rules, ingressRuleResponse{
			ID:         rule.ID,
			Port:       rule.Port,
			TargetPort: rule.TargetPort,
			CreatedAt:  rule.CreatedAt.Unix(),
		})
	}

	writeJSON(w, resp)
}

type addIngressRequest struct {
	Port       int `json:"port"`
	TargetPort int `json:"target_port"`
}

func (h *Handler) AddIngressRule(w http.ResponseWriter, r *http.Request) {
	_, userID, _, _ := getUserFromContext(r.Context())
	containerID := r.PathValue("id")

	// Verify container ownership
	container, err := h.db.GetContainer(containerID)
	if err != nil || container == nil {
		writeError(w, "container not found", http.StatusNotFound)
		return
	}
	if container.OwnerPublicID != userID {
		writeError(w, "forbidden", http.StatusForbidden)
		return
	}

	var req addIngressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate external port is allowed
	if !db.IsExternalPortAllowed(req.Port) {
		writeError(w, "external port not allowed", http.StatusBadRequest)
		return
	}

	// Default target port to same as external port if not specified
	targetPort := req.TargetPort
	if targetPort == 0 {
		targetPort = req.Port
	}

	// Validate target port
	if !db.IsTargetPortAllowed(targetPort) {
		writeError(w, "target port must be between 1 and 65535", http.StatusBadRequest)
		return
	}

	rule, err := h.db.AddIngressRule(containerID, req.Port, targetPort)
	if err != nil {
		slog.Error("failed to add ingress rule", "error", err)
		writeError(w, "failed to add ingress rule", http.StatusInternalServerError)
		return
	}

	// If port 443, also enable HTTPS routing through gateway
	if req.Port == 443 {
		if err := h.db.UpdateHTTPSEnabled(containerID, true); err != nil {
			slog.Error("failed to update https enabled", "error", err)
		}
	}

	enabledPorts := h.getEnabledPorts(containerID)

	// Update NetworkPolicy in Kubernetes
	if err := h.k8s.UpdateNetworkPolicy(r.Context(), container.Namespace, enabledPorts); err != nil {
		slog.Error("failed to update network policy", "error", err)
		// Don't fail the request, the DB is updated
	}

	// Update LoadBalancer service to expose the new port
	if err := h.k8s.UpdateLoadBalancerPorts(r.Context(), container.Namespace, h.getTargetPorts(containerID)); err != nil {
		slog.Error("failed to update load balancer ports", "error", err)
		// Don't fail the request, the DB is updated
	}

	writeJSON(w, ingressRuleResponse{
		ID:         rule.ID,
		Port:       rule.Port,
		TargetPort: rule.TargetPort,
		CreatedAt:  rule.CreatedAt.Unix(),
	})
}

func (h *Handler) RemoveIngressRule(w http.ResponseWriter, r *http.Request) {
	_, userID, _, _ := getUserFromContext(r.Context())
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
	if container.OwnerPublicID != userID {
		writeError(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := h.db.RemoveIngressRule(containerID, port); err != nil {
		slog.Error("failed to remove ingress rule", "error", err)
		writeError(w, "failed to remove ingress rule", http.StatusInternalServerError)
		return
	}

	// If port 443, also disable HTTPS routing through gateway
	if port == 443 {
		if err := h.db.UpdateHTTPSEnabled(containerID, false); err != nil {
			slog.Error("failed to update https enabled", "error", err)
		}
	}

	enabledPorts := h.getEnabledPorts(containerID)

	// Update NetworkPolicy in Kubernetes
	if err := h.k8s.UpdateNetworkPolicy(r.Context(), container.Namespace, enabledPorts); err != nil {
		slog.Error("failed to update network policy", "error", err)
		// Don't fail the request, the DB is updated
	}

	// Update LoadBalancer service to remove the port
	if err := h.k8s.UpdateLoadBalancerPorts(r.Context(), container.Namespace, h.getTargetPorts(containerID)); err != nil {
		slog.Error("failed to update load balancer ports", "error", err)
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

func (h *Handler) getTargetPorts(containerID string) []int {
	rules, err := h.db.ListIngressRules(containerID)
	if err != nil {
		return nil
	}
	var ports []int
	for _, rule := range rules {
		ports = append(ports, rule.TargetPort)
	}
	return ports
}
