package router

import (
	"os"
	"testing"
)

func testRouter(t *testing.T) *Router {
	dsn := os.Getenv("DATABASE_URL_TEST")
	if dsn == "" {
		t.Skip("DATABASE_URL_TEST not set")
	}
	r, err := New(dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	_, _ = r.db.Exec(`DELETE FROM custom_domains WHERE domain LIKE '%.test.invalid'`)
	return r
}

func TestCustomDomainCRUD(t *testing.T) {
	r := testRouter(t)
	cd := &CustomDomain{
		ID: "cd_test1", UserID: "u1", ContainerID: "c1",
		Domain: "a.test.invalid", TargetPort: 8000, VerifyToken: "tok", Status: "pending",
	}
	if err := r.CreateCustomDomain(cd); err != nil {
		t.Fatalf("CreateCustomDomain: %v", err)
	}
	got, err := r.GetCustomDomain("cd_test1")
	if err != nil || got.Domain != "a.test.invalid" {
		t.Fatalf("GetCustomDomain: %v %+v", err, got)
	}
	list, err := r.ListCustomDomainsByUser("u1")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListCustomDomainsByUser: %v len=%d", err, len(list))
	}
	if err := r.SetCustomDomainStatus("cd_test1", "verified", true); err != nil {
		t.Fatalf("SetCustomDomainStatus: %v", err)
	}
	pend, err := r.ListPendingDomains()
	if err != nil {
		t.Fatalf("ListPendingDomains: %v", err)
	}
	for _, p := range pend {
		if p.ID == "cd_test1" {
			t.Fatal("expected cd_test1 to no longer be pending")
		}
	}
	if err := r.DeleteCustomDomain("cd_test1", "u1"); err != nil {
		t.Fatalf("DeleteCustomDomain: %v", err)
	}
}

func TestResolveCustomDomain(t *testing.T) {
	r := testRouter(t)
	_, _ = r.db.Exec(`INSERT INTO containers (id, namespace, external_ip, status, user_id)
		VALUES ('cdc1','compute-u1-cdc1','10.0.0.9','running','u1')
		ON CONFLICT (id) DO UPDATE SET status='running', external_ip='10.0.0.9'`)
	cd := &CustomDomain{ID: "cd_res1", UserID: "u1", ContainerID: "cdc1",
		Domain: "live.test.invalid", TargetPort: 8000, VerifyToken: "t", Status: "verified"}
	if err := r.CreateCustomDomain(cd); err != nil {
		t.Fatalf("create: %v", err)
	}
	c, port, err := r.ResolveCustomDomain("live.test.invalid")
	if err != nil {
		t.Fatalf("ResolveCustomDomain: %v", err)
	}
	if c.Namespace != "compute-u1-cdc1" || port != 8000 {
		t.Fatalf("got ns=%s port=%d", c.Namespace, port)
	}
	if _, _, err := r.ResolveCustomDomain("nope.test.invalid"); err == nil {
		t.Fatal("expected error for unknown domain")
	}
	_ = r.DeleteCustomDomain("cd_res1", "u1")
}

func TestCustomDomainAllowed(t *testing.T) {
	r := testRouter(t)
	// The allowlist only publishes domains whose container is loaded (running),
	// so seed a running container for the domain to be allowed.
	_, _ = r.db.Exec(`INSERT INTO containers (id, namespace, external_ip, status, user_id)
		VALUES ('cda1','compute-u1-cda1','10.0.0.8','running','u1')
		ON CONFLICT (id) DO UPDATE SET status='running', external_ip='10.0.0.8'`)
	cd := &CustomDomain{ID: "cd_a1", UserID: "u1", ContainerID: "cda1",
		Domain: "allow.test.invalid", TargetPort: 8000, VerifyToken: "t", Status: "verified"}
	_ = r.CreateCustomDomain(cd)
	if !r.CustomDomainAllowed("allow.test.invalid") {
		t.Error("expected allowed")
	}
	if r.CustomDomainAllowed("missing.test.invalid") {
		t.Error("expected not allowed")
	}
	// A verified domain whose container is absent must NOT be allowlisted
	// (prevents on-demand TLS renewal for deleted containers).
	orphan := &CustomDomain{ID: "cd_orph", UserID: "u1", ContainerID: "ghost",
		Domain: "orphan.test.invalid", TargetPort: 8000, VerifyToken: "t", Status: "verified"}
	_ = r.CreateCustomDomain(orphan)
	if r.CustomDomainAllowed("orphan.test.invalid") {
		t.Error("expected orphan (missing container) to NOT be allowed")
	}
	_ = r.DeleteCustomDomain("cd_a1", "u1")
	_ = r.DeleteCustomDomain("cd_orph", "u1")
}
