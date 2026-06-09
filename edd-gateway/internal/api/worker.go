package api

import (
	"context"
	"log/slog"
	"time"

	"eddisonso.com/edd-gateway/internal/domains"
	"eddisonso.com/edd-gateway/internal/router"
)

// VerifyWorker polls pending custom domains and verifies their DNS TXT records.
type VerifyWorker struct {
	router   *router.Router
	preIssue func(string)
	interval time.Duration
}

// NewVerifyWorker builds the worker. interval of 0 defaults to 30s.
func NewVerifyWorker(r *router.Router, preIssue func(string), interval time.Duration) *VerifyWorker {
	if interval == 0 {
		interval = 30 * time.Second
	}
	return &VerifyWorker{router: r, preIssue: preIssue, interval: interval}
}

// Run polls until ctx is cancelled.
func (w *VerifyWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick()
		}
	}
}

func (w *VerifyWorker) tick() {
	pending, err := w.router.ListPendingDomains()
	if err != nil {
		slog.Error("verify worker: list pending failed", "error", err)
		return
	}
	for _, cd := range pending {
		if time.Since(cd.CreatedAt) > 7*24*time.Hour {
			if err := w.router.SetCustomDomainStatus(cd.ID, "failed", false); err != nil {
				slog.Error("verify worker: mark failed", "domain", cd.Domain, "error", err)
			}
			continue
		}
		records, _ := lookupTXT(domains.VerifyRecordName(cd.Domain))
		if domains.TXTMatches(records, cd.VerifyToken) {
			if err := w.router.SetCustomDomainStatus(cd.ID, "verified", true); err != nil {
				slog.Error("verify worker: mark verified", "domain", cd.Domain, "error", err)
				continue
			}
			slog.Info("custom domain verified", "domain", cd.Domain)
			if w.preIssue != nil {
				w.preIssue(cd.Domain)
			}
		}
	}
}
