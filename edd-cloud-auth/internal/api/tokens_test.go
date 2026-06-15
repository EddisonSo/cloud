package api

import "testing"

func TestValidateScopes_StartStop(t *testing.T) {
	err := validateScopes(map[string][]string{
		"compute.u1.containers.507b1b10": {"start", "stop"},
	}, "u1")
	if err != nil {
		t.Fatalf("start/stop on a container should be valid: %v", err)
	}
}

func TestValidateScopes_RegistryPushPullDelete(t *testing.T) {
	err := validateScopes(map[string][]string{
		"storage.u1.registry.eddison/resume": {"push", "pull", "delete"},
	}, "u1")
	if err != nil {
		t.Fatalf("registry push/pull/delete should be valid: %v", err)
	}
}

func TestValidateScopes_RejectsStartOnStorage(t *testing.T) {
	err := validateScopes(map[string][]string{
		"storage.u1.files": {"start"},
	}, "u1")
	if err == nil {
		t.Fatal("start is not a valid action for storage.files")
	}
}

func TestValidateScopes_RejectsPushOnContainers(t *testing.T) {
	err := validateScopes(map[string][]string{
		"compute.u1.containers": {"push"},
	}, "u1")
	if err == nil {
		t.Fatal("push is not a valid action for compute.containers")
	}
}

func TestValidateScopes_LegacyCRUDStillValid(t *testing.T) {
	err := validateScopes(map[string][]string{
		"compute.u1.containers":  {"create", "read", "update", "delete"},
		"storage.u1.namespaces":  {"create", "read", "update", "delete"},
		"storage.u1.files":       {"create", "read", "delete"},
		"compute.u1.keys":        {"create", "read", "delete"},
	}, "u1")
	if err != nil {
		t.Fatalf("existing CRUD scopes must remain valid: %v", err)
	}
}

func TestValidateScopes_RejectsCrossUser(t *testing.T) {
	err := validateScopes(map[string][]string{
		"compute.u2.containers": {"read"},
	}, "u1")
	if err == nil {
		t.Fatal("must reject scopes for another user's id")
	}
}

func TestValidateScopes_RegistryDottedTag(t *testing.T) {
	// Image references with dots (tags/domains) must validate — the ID segment
	// may legitimately contain dots.
	err := validateScopes(map[string][]string{
		"storage.u1.registry.foo/bar.baz": {"push"},
	}, "u1")
	if err != nil {
		t.Fatalf("dotted registry id should be valid: %v", err)
	}
}

func TestValidateScopes_Networking(t *testing.T) {
	// networking.domains and networking.domain-mappings accept create/read/delete.
	if err := validateScopes(map[string][]string{
		"networking.u1.domains":         {"create", "read", "delete"},
		"networking.u1.domain-mappings": {"create", "read", "delete"},
	}, "u1"); err != nil {
		t.Fatalf("networking scopes should be valid: %v", err)
	}
	// "update" is not an allowed action for networking resources.
	if err := validateScopes(map[string][]string{"networking.u1.domains": {"update"}}, "u1"); err == nil {
		t.Fatal("networking.domains should not accept the 'update' action")
	}
	// Unknown networking resource is rejected.
	if err := validateScopes(map[string][]string{"networking.u1.zones": {"read"}}, "u1"); err == nil {
		t.Fatal("unknown networking resource should be rejected")
	}
}

func TestValidateScopes_RootOnlyScope(t *testing.T) {
	// A root-only scope (compute.<uid>, no resource) falls back to validActions.
	if err := validateScopes(map[string][]string{"compute.u1": {"read"}}, "u1"); err != nil {
		t.Fatalf("root-only compute scope should be valid: %v", err)
	}
	if err := validateScopes(map[string][]string{"compute.u1": {"start"}}, "u1"); err == nil {
		t.Fatal("root-only scope should not accept the container-only 'start' action")
	}
}
