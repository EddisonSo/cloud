// Package tlsmgr wires certmagic for on-demand Let's Encrypt issuance, gated by
// the router's verified-domain allowlist, with certs persisted in Postgres.
package tlsmgr

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"

	"github.com/caddyserver/certmagic"

	"eddisonso.com/edd-gateway/internal/certstore"
	"eddisonso.com/edd-gateway/internal/router"
)

// Manager owns the certmagic config.
type Manager struct {
	magic *certmagic.Config
}

// New constructs a Manager backed by Postgres-persisted certs and on-demand
// Let's Encrypt issuance gated by the router's verified-domain allowlist.
func New(dsn string, r *router.Router) (*Manager, error) {
	store, err := certstore.New(dsn)
	if err != nil {
		return nil, fmt.Errorf("cert store: %w", err)
	}

	var cfg *certmagic.Config
	cache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) {
			return cfg, nil // shared config
		},
	})

	cfg = certmagic.New(cache, certmagic.Config{
		Storage: store,
		OnDemand: &certmagic.OnDemandConfig{
			DecisionFunc: func(ctx context.Context, name string) error {
				if r.CustomDomainAllowed(name) {
					return nil
				}
				return fmt.Errorf("domain %q not allowed", name)
			},
		},
	})

	ca := certmagic.LetsEncryptProductionCA
	if os.Getenv("ACME_STAGING") == "true" {
		ca = certmagic.LetsEncryptStagingCA
	}
	issuer := certmagic.NewACMEIssuer(cfg, certmagic.ACMEIssuer{
		CA:     ca,
		Email:  os.Getenv("ACME_EMAIL"),
		Agreed: true,
	})
	cfg.Issuers = []certmagic.Issuer{issuer}

	slog.Info("TLS manager initialized", "ca", ca, "staging", os.Getenv("ACME_STAGING") == "true")
	return &Manager{magic: cfg}, nil
}

// GetCertificate returns certmagic's on-demand certificate for a handshake.
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return m.magic.GetCertificate(hello)
}

// PreIssue obtains a cert for a freshly-verified domain so the first visit is fast.
func (m *Manager) PreIssue(domain string) {
	go func() {
		if err := m.magic.ManageAsync(context.Background(), []string{domain}); err != nil {
			slog.Warn("pre-issue failed", "domain", domain, "error", err)
		}
	}()
}
