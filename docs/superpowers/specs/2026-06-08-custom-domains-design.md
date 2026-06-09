# Bring Your Own Hostname (Custom Domains)

**Date:** 2026-06-08
**Status:** Approved
**Service:** edd-gateway (data + management plane), edd-cloud-interface (UI tab)

## Problem

Users can only reach their containers at the platform-generated hostname
`<container-id>.compute.cloud.eddisonso.com`. They want to point their own
domain (`abc.com`) at a container — prove they own it, point DNS at the
cluster, and have HTTPS "just work" with a valid certificate.

This requires three things the platform doesn't have today:

1. A way for a user to **claim** a domain and prove ownership.
2. **Routing** an arbitrary hostname to the right container/port.
3. A **valid TLS certificate** for a domain the platform doesn't own (the
   current setup is a single Cloudflare-DNS01 wildcard for
   `*.cloud.eddisonso.com`, which cannot cover arbitrary user domains).

## Design Decisions (settled during brainstorming)

- **DNS pointing:** support both — CNAME for subdomains
  (`www.abc.com` → a stable host that A-records to the ingress IP) and A record
  for apex (`abc.com` → ingress IP). User's choice, matching Vercel/Fly/Render.
- **Ownership proof:** DNS TXT token (`_edd-verify.abc.com = <token>`),
  decoupled from the traffic-pointing record.
- **Routing target:** a verified domain maps to `(container_id, target_port)` —
  a sibling of the existing `ingress_rules` model.
- **TLS:** on-demand ACME (Let's Encrypt) **inside edd-gateway** via the
  `certmagic` library, gated by an allowlist of verified domains. No per-domain
  Kubernetes objects.
- **Ownership of the feature:** entirely in **edd-gateway** — table, management
  API, verification worker, routing, and TLS. The gateway gains a new
  JWT-authenticated API surface (it has none today).
- **UI:** a **Networking tab** in the existing dashboard SPA
  (`edd-cloud-interface`) that calls the gateway's API. Thin React view; all
  logic in the gateway.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ edd-cloud-interface (dashboard SPA)                          │
│   Networking tab  ──HTTPS+JWT──▶  net.cloud.eddisonso.com    │
└─────────────────────────────────────────────────────────────┘
                                       │ (gateway routes this host
                                       │  to its own internal API mux)
┌──────────────────────────────────────▼──────────────────────┐
│ edd-gateway                                                  │
│                                                              │
│  Management plane (NEW)                                      │
│   • JWT middleware (validate auth-service token)             │
│   • Domains API: list/create/delete/verify                  │
│   • Verification worker: poll DNS TXT, flip pending→verified │
│                                                              │
│  Data plane                                                  │
│   • Domain resolver: host → (container, port)   [in-mem map] │
│   • On-demand TLS: certmagic GetCertificate + DecisionFunc   │
│                                                              │
│  Storage: Postgres (custom_domains + certmagic cert store)   │
└──────────────────────────────────────────────────────────────┘
```

The gateway already loads containers from Postgres into an in-memory map
refreshed on an interval (`internal/router/router.go`). Custom domains reuse
that exact pattern: a periodically-refreshed `map[string]*CustomDomain` keyed by
hostname, consulted by both the router and the TLS `DecisionFunc`.

## 1. Data model & state machine

New table in the gateway's Postgres (same DB as `containers`, `ingress_rules`,
`static_routes`):

```sql
CREATE TABLE custom_domains (
    id           TEXT PRIMARY KEY,            -- nanoid
    user_id      TEXT NOT NULL,               -- owner, from JWT
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    domain       TEXT NOT NULL UNIQUE,        -- e.g. abc.com (lowercased)
    target_port  INTEGER NOT NULL,            -- container port to route to
    verify_token TEXT NOT NULL,               -- random, for _edd-verify TXT
    status       TEXT NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    verified_at  TIMESTAMPTZ
);
CREATE INDEX idx_custom_domains_status ON custom_domains(status);
CREATE INDEX idx_custom_domains_user   ON custom_domains(user_id);
```

State machine:

| Status | Meaning | Transition |
|---|---|---|
| `pending` | row created, awaiting `_edd-verify` TXT | → `verified` when TXT matches; → `failed` after retry window |
| `verified` | DNS TXT matched; eligible for routing + cert issuance | → `active` on first successful cert issuance |
| `active` | cert issued and served at least once | (cosmetic; UI "live" badge) |
| `failed` | verification did not resolve in window | → `pending` on user retry |

**The allowlist for on-demand TLS is `status IN ('verified','active')`.** This is
the abuse gate: the gateway will only attempt Let's Encrypt issuance for domains
a user has proven they own, preventing rate-limit abuse and cross-user claims.

`domain` is `UNIQUE`, so two users cannot both claim `abc.com`; the second
`INSERT` fails. Ownership of the *container* is checked against the JWT `user_id`
at create time.

## 2. Management API (new, in edd-gateway)

Exposed on a new internal `net/http` mux (mirroring the existing health server
in `main.go:startHealthServer`), reachable via a new static route
`net.cloud.eddisonso.com` → gateway's own API listener. All routes require a
valid JWT.

| Method & path | Behavior |
|---|---|
| `GET /domains` | List the caller's domains with status + the TXT/CNAME instructions. |
| `POST /domains` | Body: `{container_id, domain, target_port}`. Verify caller owns `container_id`; validate domain syntax + port is in the container's allowed range; generate `verify_token`; insert `pending`; return setup instructions. |
| `POST /domains/{id}/verify` | Trigger an immediate DNS TXT check for one domain (in addition to the background poller). |
| `DELETE /domains/{id}` | Verify ownership; delete row; the resolver drops it on next refresh. (Cert cleanup is lazy — certmagic storage GC.) |

**JWT validation (new capability).** The gateway has no JWT code today. Add a
small middleware that validates the same token edd-cloud-auth issues — verify the
signature with the auth signing key (mounted into the gateway as a Secret, same
key/alg the auth service uses) and extract `user_id`. Mirror the claim structure
in `edd-cloud-auth`. No call-out to the auth service on the hot path; local
verification only.

**Ownership check.** `POST`/`DELETE` confirm the target container's `user_id`
matches the JWT `user_id` via a `SELECT user_id FROM containers WHERE id = $1`,
matching the existing parameterized-query style in `router.go`.

## 3. DNS-TXT verification worker

A background goroutine in the gateway (started from `main.go`, like the route
refresh loop):

- Every N seconds, `SELECT` domains in `pending`.
- For each, resolve `TXT _edd-verify.<domain>` via `net.LookupTXT`.
- If any record equals `verify_token` → `UPDATE ... SET status='verified',
  verified_at=now()`, then **kick off pre-issuance** (section 5).
- Exponential backoff per domain; after a max window (e.g. 7 days) without a
  match, mark `failed`. A manual `POST /domains/{id}/verify` resets the clock.

Use a bounded poll interval and jitter so a backlog of `pending` rows doesn't
hammer DNS. This mirrors the NATS retry/backoff discipline already used
elsewhere in the codebase.

## 4. Routing (data plane)

Add a custom-domain resolver alongside the existing container resolver in
`internal/router/router.go`:

- Extend the periodic DB load to also populate
  `customDomains map[string]*CustomDomain` (hostname → container_id +
  target_port), filtered to `status IN ('verified','active')`.
- New `ResolveCustomDomain(host string) (*Container, int, error)`:
  look up `host` in the map → get `container_id` + `target_port` → reuse the
  existing `Resolve(containerID)` path to get the `*Container` (namespace, etc.).
- In the HTTP/TLS request handlers, resolution order becomes:
  1. `*.compute.cloud.eddisonso.com` → existing container-ID extraction.
  2. Static routes (`static_routes` table) — unchanged.
  3. **Custom domain map** — NEW.
  4. Fallback (external IP) — unchanged.
- Backend dial is unchanged:
  `fmt.Sprintf("lb.%s.svc.cluster.local:%d", container.Namespace, targetPort)`
  (see `internal/proxy/http.go:142`).

## 5. On-demand TLS (the hard part)

Today `internal/proxy/server.go:LoadTLSCert` builds a static
`tls.Config{Certificates: []tls.Certificate{cert}}` — one wildcard cert, no
`GetCertificate`. This must change to a certificate-selection callback.

**Library:** `certmagic` (the engine behind Caddy's on-demand TLS). Do **not**
hand-roll an ACME client.

**Certificate selection logic** (new `GetCertificate` on the TLS config used by
`tls.Server`):

1. SNI matches `*.cloud.eddisonso.com` / `eddisonso.com` → return the existing
   statically-loaded wildcard cert (unchanged behavior).
2. Otherwise → hand to certmagic's on-demand path:
   - **`DecisionFunc(name)`**: return nil (allow) iff `name` is in the
     in-memory custom-domain allowlist (`status IN ('verified','active')`);
     else error (reject handshake). This is the abuse gate.
   - certmagic serves a cached cert if present, else obtains one from Let's
     Encrypt **in-handshake** and caches it.

**Challenge type: TLS-ALPN-01.** The gateway already terminates TLS on 443, so
TLS-ALPN-01 (answered inside the handshake via the `acme-tls/1` ALPN) needs no
extra routing. certmagic's TLS config advertises `acme-tls/1` in `NextProtos`
and intercepts those handshakes. The gateway's manual SNI pre-parse in `tls.go`
must **not** swallow `acme-tls/1` connections — they need to reach certmagic's
config. (HTTP-01 is the fallback but requires routing
`/.well-known/acme-challenge/` on port 80; avoid unless TLS-ALPN-01 is
insufficient.)

**Shared cert storage (critical for multi-replica).** Certs must NOT live in
pod-local memory/disk — multiple gateway replicas would each issue their own and
blow the Let's Encrypt rate limit (50 certs / registered domain / week).
Implement a `certmagic.Storage` backed by **Postgres** (a `certmagic_storage`
table: key TEXT PRIMARY KEY, value BYTEA, modified TIMESTAMPTZ). Reasons:
everything's already in Postgres; the same DB gives us the distributed lock
below. Use `database/sql` + `lib/pq` to match the gateway's existing DB code.

**Distributed issuance lock.** Two replicas getting the first request for
`abc.com` simultaneously must not both call ACME. Implement certmagic's `Locker`
interface using **Postgres advisory locks** (`pg_try_advisory_lock` on a hash of
the lock key), so only one replica issues while others wait/serve.

**Pre-issue on verify (the latency fix).** Issuing synchronously in the first
handshake makes the first visit to a new domain take ~2–5s or briefly fail. To
avoid that, the verification worker calls certmagic to obtain the cert the moment
a domain flips to `verified`, so it's usually cached before the first real
visitor. On-demand issuance remains the fallback for anything not pre-issued.

**Networking prerequisite.** ACME validation requires the gateway be publicly
reachable on 443 (TLS-ALPN-01) — it already is.

## 6. UI: Networking tab (edd-cloud-interface)

A new tab in the dashboard SPA. Thin view over the gateway API:

- List domains with status badges (`pending` / `verified` / `live` / `failed`).
- "Add domain" form: pick a container, enter domain, pick a target port.
- On create, show copy-paste setup instructions:
  - TXT record: `_edd-verify.<domain>` → `<verify_token>` (for verification)
  - Traffic record: CNAME `<domain>` → stable ingress host (subdomains) **or**
    A `<domain>` → ingress IP (apex).
- "Verify now" button → `POST /domains/{id}/verify`; poll status.
- Delete with confirmation.

Follows the existing responsive table/card pattern used elsewhere in the
dashboard.

## Error handling & edge cases

- **Duplicate domain claim:** `UNIQUE(domain)` rejects the second claim; API
  returns a clear "domain already in use" error.
- **Container deleted:** `ON DELETE CASCADE` removes its custom domains; resolver
  drops them on next refresh.
- **Port not exposed / out of range:** validate `target_port` against the
  container's allowed ingress range at create time (same rules as
  `ingress_rules`).
- **Verification never completes:** backoff → `failed` after the window; user can
  retry, which resets to `pending`.
- **ACME failure (DNS not pointed yet, LE outage):** `DecisionFunc` still allows
  (domain is verified), certmagic retries with its own backoff; handshake fails
  gracefully until a cert exists. Surface "cert pending" in the UI when
  `verified` but not yet `active`.
- **Let's Encrypt rate limits:** the verified-only allowlist + shared storage +
  pre-issue-once make duplicate issuance unlikely. Use the **LE staging**
  endpoint during development to avoid burning prod limits.

## Testing strategy

- **Unit:** domain syntax/port validation; TXT-match logic; resolver lookup;
  `DecisionFunc` allow/deny against a fake allowlist; JWT middleware
  (valid/expired/wrong-signature).
- **Integration:** Postgres-backed `certmagic.Storage` + advisory-lock `Locker`
  (two goroutines race the same key → exactly one issues).
- **ACME end-to-end:** against Let's Encrypt **staging** with a throwaway domain
  pointed at a test ingress; assert TLS-ALPN-01 issuance and that a second
  replica reads the cert from shared storage rather than re-issuing.
- **Routing:** request with `Host: abc.com` resolves to the mapped
  container/port and proxies correctly; unverified domain is rejected at TLS.

## Build order (de-risk TLS first)

1. **Spike the TLS layer first**, in isolation, against LE staging: certmagic +
   Postgres storage + advisory-lock Locker + TLS-ALPN-01 + `DecisionFunc`. This
   is ~70% of the risk; prove it before building around it.
2. `custom_domains` table + migration.
3. Router custom-domain resolver + in-memory map refresh.
4. JWT middleware + management API + ownership checks.
5. DNS-TXT verification worker + pre-issue-on-verify hook.
6. Networking tab in the dashboard.

## Effort / difficulty summary

| Piece | Effort | Risk |
|---|---|---|
| `custom_domains` table + migration | trivial | low |
| Router custom-domain resolver | easy (clones container lookup) | low |
| JWT middleware on gateway (new surface) | small | low–med |
| Domain CRUD API | easy | low |
| DNS-TXT verification worker | easy–moderate | low |
| **On-demand TLS (certmagic + PG storage + lock + ALPN)** | **moderate–hard, bulk of work** | **med–high** |
| Pre-issue-on-verify | small | low |
| Networking tab (React) | moderate | low |

**Overall: ~1 week of focused work.** Conceptually straightforward; ~70% of the
risk and effort is concentrated in the on-demand TLS layer. Everything else is a
variation on patterns already in the codebase.
