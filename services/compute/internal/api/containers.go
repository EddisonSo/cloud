package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"eddisonso.com/edd-cloud/services/compute/internal/db"
	"github.com/google/uuid"
)

const (
	maxContainersPerUser = 3
	defaultMemoryMB      = 512
	defaultStorageGB     = 5
	defaultImage         = "eddisonso/ecloud-compute-base:latest"
)

type containerRequest struct {
	Name      string  `json:"name"`
	MemoryMB  int     `json:"memory_mb"`
	StorageGB int     `json:"storage_gb"`
	SSHKeyIDs []int64 `json:"ssh_key_ids"`
}

type containerResponse struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Hostname      string   `json:"hostname"`
	SSHCommand    *string  `json:"ssh_command,omitempty"`
	MemoryMB      int      `json:"memory_mb"`
	MemoryUsedMB  *int64   `json:"memory_used_mb,omitempty"`
	StorageGB     int      `json:"storage_gb"`
	StorageUsedGB *float64 `json:"storage_used_gb,omitempty"`
	CreatedAt     string   `json:"created_at"`
	SSHEnabled    bool     `json:"ssh_enabled"`
	HTTPSEnabled  bool     `json:"https_enabled"`
}

func (h *Handler) ListContainers(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := getUserFromContext(r.Context())
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	containers, err := h.db.ListContainersByUser(userID)
	if err != nil {
		slog.Error("failed to list containers", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	resp := make([]containerResponse, 0, len(containers))
	for _, c := range containers {
		cr := containerToResponse(c)
		// Fetch usage for running containers
		if c.Status == "running" {
			if usage, err := h.k8s.GetResourceUsage(ctx, c.Namespace); err == nil {
				cr.MemoryUsedMB = &usage.MemoryUsedMB
				rounded := float64(int(usage.StorageUsedGB*10)) / 10
				cr.StorageUsedGB = &rounded
			}
		}
		resp = append(resp, cr)
	}

	writeJSON(w, map[string]any{"containers": resp})
}

func (h *Handler) CreateContainer(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := getUserFromContext(r.Context())
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req containerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate name
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, "name is required", http.StatusBadRequest)
		return
	}

	// Check container limit
	count, err := h.db.CountContainersByUser(userID)
	if err != nil {
		slog.Error("failed to count containers", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if count >= maxContainersPerUser {
		writeError(w, fmt.Sprintf("container limit reached (%d)", maxContainersPerUser), http.StatusBadRequest)
		return
	}

	// Validate SSH keys
	if len(req.SSHKeyIDs) == 0 {
		writeError(w, "at least one SSH key is required", http.StatusBadRequest)
		return
	}

	sshKeys, err := h.db.GetSSHKeysByIDs(userID, req.SSHKeyIDs)
	if err != nil {
		slog.Error("failed to get ssh keys", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(sshKeys) != len(req.SSHKeyIDs) {
		writeError(w, "one or more SSH keys not found", http.StatusBadRequest)
		return
	}

	// Set defaults
	memoryMB := req.MemoryMB
	if memoryMB <= 0 {
		memoryMB = defaultMemoryMB
	}
	storageGB := req.StorageGB
	if storageGB <= 0 {
		storageGB = defaultStorageGB
	}

	// Generate container ID and namespace
	containerID := uuid.New().String()[:8]
	namespace := fmt.Sprintf("compute-%d-%s", userID, containerID)

	// Create container record
	container := &db.Container{
		ID:        containerID,
		UserID:    userID,
		Name:      req.Name,
		Namespace: namespace,
		Status:    "pending",
		MemoryMB:  memoryMB,
		StorageGB: storageGB,
		Image:     defaultImage,
	}

	if err := h.db.CreateContainer(container); err != nil {
		slog.Error("failed to create container record", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Create K8s resources in background
	go h.provisionContainer(container, sshKeys)

	writeJSON(w, containerToResponse(container))
}

func (h *Handler) provisionContainer(container *db.Container, sshKeys []*db.SSHKey) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Build authorized_keys with user's keys
	var authorizedKeys strings.Builder
	for _, key := range sshKeys {
		authorizedKeys.WriteString(key.PublicKey)
		authorizedKeys.WriteString("\n")
	}

	// Add gateway's public key if available (enables SSH routing via gateway)
	if gatewayKey, err := h.k8s.GetGatewayPublicKey(ctx); err == nil && gatewayKey != "" {
		authorizedKeys.WriteString("# Gateway key for SSH routing\n")
		authorizedKeys.WriteString(gatewayKey)
		if !strings.HasSuffix(gatewayKey, "\n") {
			authorizedKeys.WriteString("\n")
		}
	}

	// Create namespace
	if err := h.k8s.CreateNamespace(ctx, container.Namespace, container.UserID, container.ID); err != nil {
		slog.Error("failed to create namespace", "container", container.ID, "error", err)
		h.db.UpdateContainerStatus(container.ID, "failed")
		GetHub().SendContainerStatus(container.UserID, container.ID, "failed", nil)
		return
	}

	// Create SSH secret
	if err := h.k8s.CreateSSHSecret(ctx, container.Namespace, authorizedKeys.String()); err != nil {
		slog.Error("failed to create ssh secret", "container", container.ID, "error", err)
		h.db.UpdateContainerStatus(container.ID, "failed")
		GetHub().SendContainerStatus(container.UserID, container.ID, "failed", nil)
		return
	}

	// Create PVC
	if err := h.k8s.CreatePVC(ctx, container.Namespace, container.StorageGB); err != nil {
		slog.Error("failed to create pvc", "container", container.ID, "error", err)
		h.db.UpdateContainerStatus(container.ID, "failed")
		GetHub().SendContainerStatus(container.UserID, container.ID, "failed", nil)
		return
	}

	// Create NetworkPolicy
	if err := h.k8s.CreateNetworkPolicy(ctx, container.Namespace); err != nil {
		slog.Error("failed to create network policy", "container", container.ID, "error", err)
		h.db.UpdateContainerStatus(container.ID, "failed")
		GetHub().SendContainerStatus(container.UserID, container.ID, "failed", nil)
		return
	}

	// Create Pod
	if err := h.k8s.CreatePod(ctx, container.Namespace, container.Image, container.MemoryMB); err != nil {
		slog.Error("failed to create pod", "container", container.ID, "error", err)
		h.db.UpdateContainerStatus(container.ID, "failed")
		GetHub().SendContainerStatus(container.UserID, container.ID, "failed", nil)
		return
	}

	// Create LoadBalancer service
	if err := h.k8s.CreateLoadBalancer(ctx, container.Namespace); err != nil {
		slog.Error("failed to create load balancer", "container", container.ID, "error", err)
		h.db.UpdateContainerStatus(container.ID, "failed")
		GetHub().SendContainerStatus(container.UserID, container.ID, "failed", nil)
		return
	}

	// Resources created, status is "initializing" until pod is ready
	h.db.UpdateContainerStatus(container.ID, "initializing")
	slog.Info("container resources created", "container", container.ID, "namespace", container.Namespace)
	GetHub().SendContainerStatus(container.UserID, container.ID, "initializing", nil)

	// Poll for pod readiness and external IP
	go h.pollContainerReady(container)
}

func (h *Handler) pollContainerReady(container *db.Container) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	podReady := false
	ipAssigned := false
	var externalIP string

	for {
		select {
		case <-ctx.Done():
			slog.Warn("timeout waiting for container ready", "container", container.ID)
			return
		case <-ticker.C:
			// Check pod status
			if !podReady {
				status, err := h.k8s.GetPodStatus(ctx, container.Namespace)
				if err != nil {
					slog.Error("failed to get pod status", "container", container.ID, "error", err)
					continue
				}

				if status == "running" {
					podReady = true
					h.db.UpdateContainerStatus(container.ID, "running")
					slog.Info("container running", "container", container.ID)
					GetHub().SendContainerStatus(container.UserID, container.ID, "running", nil)
				} else if status == "failed" {
					h.db.UpdateContainerStatus(container.ID, "failed")
					GetHub().SendContainerStatus(container.UserID, container.ID, "failed", nil)
					return
				}
			}

			// Check for external IP
			if !ipAssigned {
				ip, err := h.k8s.GetServiceExternalIP(ctx, container.Namespace)
				if err != nil {
					slog.Error("failed to get external ip", "container", container.ID, "error", err)
					continue
				}
				if ip != "" {
					ipAssigned = true
					externalIP = ip
					if err := h.db.UpdateContainerIP(container.ID, ip); err != nil {
						slog.Error("failed to update container ip", "container", container.ID, "error", err)
					}
					slog.Info("external IP assigned", "container", container.ID, "ip", ip)
					// Broadcast IP assignment
					currentStatus := "initializing"
					if podReady {
						currentStatus = "running"
					}
					GetHub().SendContainerStatus(container.UserID, container.ID, currentStatus, &externalIP)
				}
			}

			// Done when both pod is ready and IP assigned
			if podReady && ipAssigned {
				return
			}
		}
	}
}

func (h *Handler) GetContainer(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := getUserFromContext(r.Context())
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	containerID := r.PathValue("id")
	container, err := h.db.GetContainer(containerID)
	if err != nil {
		slog.Error("failed to get container", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if container == nil || container.UserID != userID {
		writeError(w, "container not found", http.StatusNotFound)
		return
	}

	// Get current pod status from K8s
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	status, err := h.k8s.GetPodStatus(ctx, container.Namespace)
	if err == nil && status != "" && status != container.Status {
		container.Status = status
		h.db.UpdateContainerStatus(container.ID, status)
	}

	// Check for IP if not yet assigned
	if !container.ExternalIP.Valid {
		ip, err := h.k8s.GetServiceExternalIP(ctx, container.Namespace)
		if err == nil && ip != "" {
			container.ExternalIP.String = ip
			container.ExternalIP.Valid = true
			h.db.UpdateContainerIP(container.ID, ip)
		}
	}

	resp := containerToResponse(container)
	// Fetch usage for running containers
	if container.Status == "running" {
		if usage, err := h.k8s.GetResourceUsage(ctx, container.Namespace); err == nil {
			resp.MemoryUsedMB = &usage.MemoryUsedMB
			rounded := float64(int(usage.StorageUsedGB*10)) / 10
			resp.StorageUsedGB = &rounded
		}
	}

	writeJSON(w, resp)
}

func (h *Handler) DeleteContainer(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := getUserFromContext(r.Context())
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	containerID := r.PathValue("id")
	container, err := h.db.GetContainer(containerID)
	if err != nil {
		slog.Error("failed to get container", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if container == nil || container.UserID != userID {
		writeError(w, "container not found", http.StatusNotFound)
		return
	}

	// Delete namespace (will cascade delete all resources)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := h.k8s.DeleteNamespace(ctx, container.Namespace); err != nil {
		slog.Error("failed to delete namespace", "error", err)
		writeError(w, "failed to delete container", http.StatusInternalServerError)
		return
	}

	if err := h.db.DeleteContainer(containerID); err != nil {
		slog.Error("failed to delete container record", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) StopContainer(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := getUserFromContext(r.Context())
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	containerID := r.PathValue("id")
	container, err := h.db.GetContainer(containerID)
	if err != nil {
		slog.Error("failed to get container", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if container == nil || container.UserID != userID {
		writeError(w, "container not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := h.k8s.DeletePod(ctx, container.Namespace); err != nil {
		slog.Error("failed to delete pod", "error", err)
		writeError(w, "failed to stop container", http.StatusInternalServerError)
		return
	}

	if err := h.db.UpdateContainerStopped(containerID); err != nil {
		slog.Error("failed to update container status", "error", err)
	}

	container.Status = "stopped"
	// Broadcast stopped status via WebSocket
	GetHub().SendContainerStatus(container.UserID, container.ID, "stopped", nil)
	writeJSON(w, containerToResponse(container))
}

func (h *Handler) StartContainer(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := getUserFromContext(r.Context())
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	containerID := r.PathValue("id")
	container, err := h.db.GetContainer(containerID)
	if err != nil {
		slog.Error("failed to get container", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if container == nil || container.UserID != userID {
		writeError(w, "container not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := h.k8s.CreatePod(ctx, container.Namespace, container.Image, container.MemoryMB); err != nil {
		slog.Error("failed to create pod", "error", err)
		writeError(w, "failed to start container", http.StatusInternalServerError)
		return
	}

	if err := h.db.UpdateContainerStatus(containerID, "pending"); err != nil {
		slog.Error("failed to update container status", "error", err)
	}

	container.Status = "pending"
	// Broadcast pending status via WebSocket
	GetHub().SendContainerStatus(container.UserID, container.ID, "pending", nil)

	// Poll for container to become ready
	go h.pollContainerReady(container)

	writeJSON(w, containerToResponse(container))
}

func containerToResponse(c *db.Container) containerResponse {
	// Use first 8 chars of container ID for hostname
	shortID := c.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	hostname := fmt.Sprintf("%s.compute.eddisonso.com", shortID)

	resp := containerResponse{
		ID:           c.ID,
		Name:         c.Name,
		Status:       c.Status,
		Hostname:     hostname,
		MemoryMB:     c.MemoryMB,
		StorageGB:    c.StorageGB,
		CreatedAt:    c.CreatedAt.Format(time.RFC3339),
		SSHEnabled:   c.SSHEnabled,
		HTTPSEnabled: c.HTTPSEnabled,
	}

	if c.ExternalIP.Valid {
		sshCmd := fmt.Sprintf("ssh root@%s", hostname)
		resp.SSHCommand = &sshCmd
	}

	return resp
}
