# Cloudflare DNS Automation for Custom Domains (Per-User Tokens)

**Date:** 2026-06-09
**Status:** Approved (rev 2 — per-user tokens; rev 1's platform-wide token was dropped)
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

### 1. Token storage — encrypted at rest in the gateway's Postgres

```sql
CREATE TABLE IF NOT EXISTS user_cloudflare_tokens (
    user_id          TEXT PRIMARY KEY,
    token_ciphertext BYTEA NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

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

### 3. Token management API (existing authenticated mux, JWT + CORS)

- `PUT /api/cloudflare-token` body `{token}` — **validate on save** by calling
  `ListZones` with it; failure → 400 "token invalid or lacks zone access";
  success → seal + upsert row, respond `{configured: true, zones: [names]}`.
- `GET /api/cloudflare-token` → `{configured: bool, zones?: [names]}` (zones
  fetched live when configured; the token itself is never returned).
- `DELETE /api/cloudflare-token` → remove the row, 204.
- All three return 503 if `TOKEN_ENCRYPTION_KEY` is unset.

### 4. createDomain flow

After existing validation (syntax, port, container ownership):

```
token := load+decrypt requesting user's CF token (if box configured)
if token exists:
    cf := cloudflare.New(token)
    zoneID, err := cf.FindZone(domain)
    if err == nil && cf.UpsertCNAME(zoneID, domain, "ingress.cloud.eddisonso.com") == nil:
        insert status='verified' (verified_at stamped in the INSERT)
        preIssue(domain)
        respond 201 {.., dns_automated: true}
        return
    // ErrZoneNotFound or any error: log warning, fall through
insert status='pending'; respond with TXT instructions  // manual path, unchanged
```

`DELETE /domains/{id}`: after the DB delete, best-effort guarded
`DeleteRecord` using that user's token; failures log a warning only.

`dns_automated` is response-only — no change to the `custom_domains` schema.

### 5. Frontend (Networking tab)

- New "Cloudflare integration" card: paste token → save → shows
  "Connected — zones: …" on success; disconnect button. Helper text tells the
  user to create a token scoped to `Zone:Read` + `DNS:Edit` on just their zone.
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
