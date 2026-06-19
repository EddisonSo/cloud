package api

import (
	"testing"

	"eddisonso.com/edd-cloud/services/compute/internal/db"
)

func TestContainerToResponse_IncludesPullPolicy(t *testing.T) {
	c := &db.Container{ID: "abc123", PullPolicy: "Always"}
	resp := containerToResponse(c)
	if resp.PullPolicy != "Always" {
		t.Errorf("PullPolicy = %q, want Always", resp.PullPolicy)
	}
}
