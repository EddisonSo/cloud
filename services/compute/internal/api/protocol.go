package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type sshAccessResponse struct {
	SSHEnabled bool `json:"ssh_enabled"`
}

func (h *Handler) GetSSHAccess(w http.ResponseWriter, r *http.Request) {
	userID, _, _, _ := getUserFromContext(r.Context())
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

	writeJSON(w, sshAccessResponse{
		SSHEnabled: container.SSHEnabled,
	})
}

type updateSSHAccessRequest struct {
	SSHEnabled bool `json:"ssh_enabled"`
}

func (h *Handler) UpdateSSHAccess(w http.ResponseWriter, r *http.Request) {
	userID, _, _, _ := getUserFromContext(r.Context())
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

	var req updateSSHAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Update database
	if err := h.db.UpdateSSHEnabled(containerID, req.SSHEnabled); err != nil {
		slog.Error("failed to update ssh access", "error", err)
		writeError(w, "failed to update ssh access", http.StatusInternalServerError)
		return
	}

	writeJSON(w, sshAccessResponse{
		SSHEnabled: req.SSHEnabled,
	})
}
