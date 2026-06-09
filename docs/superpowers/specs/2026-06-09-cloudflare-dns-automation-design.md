# Cloudflare DNS Automation for Custom Domains (Per-User Tokens)

**Date:** 2026-06-09
**Status:** Approved (rev 3 — multiple connections per user; rev 2 allowed only one
token per user, forcing a connect→map→disconnect→connect dance when zones live
in separately-scoped tokens; rev 1's platform-wide token was dropped)
**Service:** edd-gateway (+ Networking tab touch)

## Problem

Adding a custom domain requires the user to manually create DNS records. Two
failure modes showed up on day one:

1. The user creates the traffic record **proxied** (Cloudflare orange-cloud),
   which breaks the feature twice: ACME TLS-ALPN-01 challenges terminate at
   Cloudflare's edge (no cert ever issues), and Cloudflare's plaintext origin
   leg meets the gateway's HTTP→HTTPS redirect in an infinite 301 loop.
2. The manual record dance is slow and error-prone even when done right.

Since the domain owner controls their DNS zone, they can hand the platform a
scoped Cloudflare API token and the platform creates the records itself —
correctly (`proxied: false` forced), instantly, with verification implied.

## Decisions

- **Per-user tokens.** The end user bringing the domain provides their own
  Cloudflare API token (scoped `Zone:Read` + `Zone.DNS:Edit`, ideally to just
  their zone). The platform never holds a global DNS credential. Users who
  don't want to share a token keep the manual TXT flow.
- **Cloudflare only.** No multi-provider abstraction. Non-Cloudflare domains
  use the manual flow.
- **Auto-verify.** A working DNS-edit token for the zone *is* proof of
  control: when the platform successfully creates the CNAME, the domain is
  marked `verified` immediately and cert pre-issuance fires. Add → live in
  seconds.
- **Graceful fallback everywhere.** No token stored, zone not on the token,
  CF API error, encryption key unset → degrade to the manual `pending` + TXT
  path. Cloudflare problems must never block adding a domain.
- **Platform DNS needs nothing.** `ingress.cloud.eddisonso.com` (the CNAME
  target) is already maintained by the DDNS `*.cloud.eddisonso.com` wildcard.

## Design

### 1. Token storage — multiple connections per user, encrypted at rest

A user holds any number of **connections**, each one Cloudflare API token
(typically scoped to a single zone — tighter blast radius than one broad
token). Connections accumulate; there is no connect/disconnect dance to switch
zones.

```sql
CREATE TABLE IF NOT EXISTS cloudflare_connections (
    id               TEXT PRIMARY KEY,            -- cfc_<random>
    user_id          TEXT NOT NULL,
    token_ciphertext BYTEA NOT NULL,
    zones            TEXT[] NOT NULL DEFAULT '{}', -- zone names snapshot
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON cloudflare_connections(user_id);
```

`zones` is snapshotted at connect time (from the save-time ListZones
validation) and used both for display and to pick the right connection when a
domain is added. A per-connection **refresh** re-snapshots (token scopes can be
edited in Cloudflare without rolling the secret, and "all zones" tokens gain
zones when the account does).

**Migration from rev 2:** at startup, if the old `user_cloudflare_tokens`
table exists, its rows are copied into `cloudflare_connections` (deterministic
ids, empty zones) and the old table is dropped. Empty-zones connections are
lazily backfilled on the first GET (live ListZones, persisted).

- Tokens are sealed with **AES-256-GCM** using a 32-byte platform key from K8s
  Secret `gateway-token-key` (env `TOKEN_ENCRYPTION_KEY`, hex-encoded,
  generated with `openssl rand -hex 32`, created via `kubectl create secret` —
  never committed).
- Plaintext tokens exist only in request bodies and process memory. Never
  logged, never persisted unencrypted, never returned by any endpoint.
- Env key unset → the whole integration is disabled (token endpoints return
  503 "not configured"; createDomain skips automation).
- New small package `internal/secretbox`: `New(keyHex string) (*Box, error)`,
  `Seal(plaintext []byte) []byte`, `Open(ciphertext []byte) ([]byte, error)`
  (random nonce prepended to ciphertext).

### 2. Cloudflare client — `edd-gateway/internal/cloudflare/`

Plain `net/http` against the CF v4 API; per-request client built from the
user's decrypted token (`cloudflare.New(token)`). Methods:

- `ListZones() ([]Zone, error)` — `GET /zones?per_page=50`; `Zone{ID, Name}`
  exported (used for save-time validation feedback and zone matching).
- `FindZone(domain) (zoneID, error)` — longest dot-suffix match over
  `ListZones`; sentinel `ErrZoneNotFound`.
- `UpsertCNAME(zoneID, name, target) error` — find-by-name then POST (create)
  or PUT (update); always `type=CNAME`, `proxied: false`, `ttl: 1`. Upsert
  deliberately **repairs** a pre-existing orange-cloud record.
- `DeleteRecord(zoneID, name, expectedContent) error` — deletes only if the
  record's content equals `expectedContent` (the ingress target), so a record
  the user repurposed is never destroyed.

All calls send `Authorization: Bearer <token>`; CF's `{success, errors,
result}` envelope is decoded and `success:false` surfaces as an error.

### 3. Connections API (existing authenticated mux, JWT + CORS)

- `POST /api/cloudflare-connections` body `{token}` — **validate on save** via
  `ListZones`; failure → 400 "token invalid or lacks zone access"; success →
  seal + insert with the zones snapshot, respond 201
  `{id, zones, created_at}`.
- `GET /api/cloudflare-connections` → `{connections: [{id, zones,
  created_at}]}` (tokens never returned; empty-zones rows are lazily
  backfilled here).
- `POST /api/cloudflare-connections/{id}/refresh` — re-run ListZones with the
  stored token, persist and return the new snapshot.
- `DELETE /api/cloudflare-connections/{id}` — ownership-checked, 204. Existing
  domains and DNS records are untouched.
- All return 503 if `TOKEN_ENCRYPTION_KEY` is unset. (Rev 2's
  `/api/cloudflare-token` endpoint is removed — the dashboard is the only
  consumer and deploys in lockstep.)

### 4. createDomain flow

After existing validation (syntax, port, container ownership):

```
insert row status='pending'         // FIRST — a duplicate-domain 409 exits here
if insert failed: respond 409/500   // before any DNS write can happen
conn := the user's connection whose stored zones best match the domain
        (longest dot-suffix across ALL connections; nil if none match)
if conn != nil:
    cf := cloudflare.New(decrypt(conn.token))
    zoneID, err := cf.FindZone(domain)   // live revalidation + zone ID
    if err == nil && cf.UpsertCNAME(zoneID, domain, "ingress.cloud.eddisonso.com") == nil:
        SetCustomDomainStatus(id, 'verified')   // stamps verified_at
        preIssue(domain)
        respond 201 {.., dns_automated: true}
        return
    // ErrZoneNotFound or any error: log warning, fall through
respond 201 with TXT instructions   // manual path, unchanged
```

Domain-delete cleanup picks the connection the same way (stored-zone match)
before the guarded `DeleteRecord`. If no connection's zones match (e.g. stale
snapshot), the manual path applies — the user can refresh the connection and
retry.

Ordering matters: the upsert deliberately overwrites whatever record sits at
the name, so it must never run for a create that fails — otherwise a 409 could
rewrite the user's zone (and even repoint their hostname at another user's
container) while reporting an error.

`DELETE /domains/{id}`: after the DB delete, best-effort guarded
`DeleteRecord` using that user's token; failures log a warning only.

`dns_automated` is response-only — no change to the `custom_domains` schema.

### 5. Frontend (Networking → Domains sub-tab)

- **Nav restructure:** Networking gains a sub-item **Domains** (same pattern
  as Compute → Containers / SSH Keys): nav entry `/networking/domains`,
  `/networking` redirects there. The existing page content moves under it,
  leaving room for future sub-tabs.
- **Cloudflare connections card** (replaces the single-token card): lists each
  connection with its zones, a **Refresh zones** action, and a **Disconnect**
  button — plus an always-visible "Add connection" token input. Connecting
  `eddisonso.com` and `eddisonso2.com` as two separately-scoped tokens and
  mapping hostnames under both, with no disconnect dance, is the acceptance
  test.
- Add-domain flow unchanged; on a `dns_automated: true` response the form
  shows "DNS configured automatically — your domain is going live" and the
  TXT instructions card never appears (status is already `verified`).

### 6. Unchanged

Verify worker (still serves manual-path domains), on-demand TLS/certmagic,
router resolution, CORS middleware, `custom_domains` schema.

## Error handling

| Failure | Behavior |
|---|---|
| `TOKEN_ENCRYPTION_KEY` unset | Token endpoints 503; createDomain skips automation |
| User has no stored token | Manual flow (today's behavior) |
| Token invalid at save time | 400, not stored |
| Token revoked later / CF 5xx during create | Log warning → manual flow for that create |
| Domain's zone not on the user's token | `ErrZoneNotFound` → manual flow |
| Existing record at same name | Upsert overwrites to grey-cloud CNAME → ingress (repairs misconfig) |
| Delete: CF cleanup fails / content ≠ ingress target | Log / leave record; domain delete still succeeds |
| Decryption failure (key rotated) | Treat as no token; log warning |

## Security

- Custodianship of user DNS-edit credentials is the main new risk. Mitigations:
  AES-256-GCM at rest, key only in a K8s Secret, plaintext never logged or
  returned, save-time scoping guidance in the UI, one-click disconnect.
- Auto-verify does not weaken the abuse gate: automation only succeeds for
  zones the *requesting user's own token* controls — claiming someone else's
  domain still requires the TXT proof.

## Testing

- **Unit:** secretbox round-trip + tamper detection; zone suffix matching; CF
  client against `httptest` stubs (create vs update, `proxied:false` payload,
  delete guard, error envelope); token-endpoint validation paths.
- **DB-gated:** token store CRUD (`DATABASE_URL_TEST`).
- **End-to-end (manual):** connect token in the tab → re-add
  `resume.eddisonso2.com` → record appears grey-cloud in CF (repairing the
  currently-proxied one), domain verified instantly, prod cert issues,
  HTTPS 200.

## Effort

Small-to-medium: one crypto helper (~60 lines), one CF client (~150 lines),
one DB table + CRUD, three token endpoints + a guarded branch in createDomain,
manifest env, a frontend card. TLS/routing untouched.
