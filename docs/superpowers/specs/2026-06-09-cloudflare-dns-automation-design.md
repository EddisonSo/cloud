# Cloudflare DNS Automation for Custom Domains

**Date:** 2026-06-09
**Status:** Approved
**Service:** edd-gateway (+ small frontend touch)

## Problem

Adding a custom domain currently requires the user to manually create two DNS
records (the `_edd-verify` TXT and the traffic CNAME). Two failure modes showed
up in practice on day one:

1. The user creates the CNAME **proxied** (Cloudflare orange-cloud), which
   breaks the feature twice over: ACME TLS-ALPN-01 challenges hit Cloudflare's
   edge instead of the gateway (no cert ever issues), and Cloudflare's
   plaintext origin leg meets the gateway's HTTP→HTTPS redirect in an infinite
   301 loop.
2. The manual two-record dance is slow and error-prone even when done right.

Since the operator's zones live on Cloudflare, the platform can create the
records itself via the Cloudflare API — correctly (`proxied: false`), instantly,
and with verification implied.

## Decisions (settled during brainstorming)

- **Cloudflare only.** No multi-provider abstraction. Domains on other DNS
  providers keep the existing manual TXT flow.
- **Single platform-wide API token** (operator's Cloudflare account), scoped to
  `Zone:Read` + `Zone.DNS:Edit`, stored as a Kubernetes Secret — never in the
  DB, ConfigMaps, or logs.
- **Auto-verify.** Holding a DNS-edit token for the zone *is* proof of control.
  When automation succeeds, the domain is marked `verified` immediately and
  cert pre-issuance fires — no TXT record, no poll wait. Add → live in seconds.
- **Graceful fallback.** Token unset, zone not found on the token, or any CF
  API error → the flow degrades to today's manual `pending` + TXT-instructions
  path. Cloudflare failures must never block adding a domain.

## Design

### 1. Token & config

- K8s Secret `cloudflare-api` in namespace `core`, key `CLOUDFLARE_API_TOKEN`,
  created with `kubectl create secret` (never committed).
- Gateway Deployment gets the env var via `secretKeyRef` (same pattern as
  `JWT_SECRET`). Env var absent → automation disabled, everything behaves as
  today.

### 2. Cloudflare client — `edd-gateway/internal/cloudflare/`

Plain `net/http` against the Cloudflare v4 API (three endpoints — no SDK
dependency). All calls send `Authorization: Bearer <token>`.

- `FindZone(domain string) (zoneID string, err error)` — `GET /zones`, match
  the **longest zone-name suffix** of the domain
  (`resume.eddisonso2.com` → zone `eddisonso2.com`). No match → sentinel
  `ErrZoneNotFound` (caller falls back to manual flow).
- `UpsertCNAME(zoneID, name, target string) error` — look up an existing
  record for `name` (`GET /zones/{id}/dns_records?name=`); `POST` to create or
  `PUT` to update, always `type=CNAME`, `content=target`, **`proxied: false`**.
  Upsert semantics deliberately *repair* a pre-existing orange-cloud record.
- `DeleteRecord(zoneID, name string) error` — find record by name, `DELETE` it.
  Used best-effort on domain deletion.

The target is the existing stable ingress host `ingress.cloud.eddisonso.com`
(covered by the DDNS-maintained wildcard).

### 3. createDomain flow (edd-gateway/internal/api/server.go)

After the existing validation (domain syntax, port, container ownership):

```
if cfClient != nil {
    zoneID, err := cfClient.FindZone(domain)
    if err == nil {
        if err := cfClient.UpsertCNAME(zoneID, domain, ingressHost); err == nil {
            insert row with status='verified', verified_at=now()
            preIssue(domain)
            respond 201 with dns_automated: true
            return
        }
    }
    // ErrZoneNotFound or any CF error: log and fall through
}
// manual path, unchanged: insert status='pending', respond with TXT instructions
```

- `dns_automated` is **response-only** — no schema change to `custom_domains`.
- CF errors are logged (`slog.Warn`) with the domain but never the token.

`DELETE /domains/{id}`: after the DB delete succeeds, best-effort
`FindZone` + `DeleteRecord` for the domain's CNAME. Failure logs a warning and
does not affect the API response. (Conservative: only deletes a record whose
content is exactly `ingress.cloud.eddisonso.com`, so an unrelated record the
user repurposed is never destroyed.)

### 4. Unchanged

Verify worker (still serves manual-path domains), on-demand TLS/certmagic,
router resolution, `custom_domains` schema, CORS middleware.

### 5. Frontend (Networking tab)

- `CustomDomain`/create response type gains optional `dns_automated?: boolean`.
- When `dns_automated` is true, the domain card shows
  "DNS configured automatically — going live" instead of the TXT/CNAME setup
  instructions, and the status badge proceeds Verified → Live as usual.
- Manual-path rendering unchanged.

## Error handling

| Failure | Behavior |
|---|---|
| `CLOUDFLARE_API_TOKEN` unset | Automation disabled; manual flow for everyone |
| Domain's zone not on the token | `ErrZoneNotFound` → manual flow for that domain |
| CF API down / 5xx / rate limit | Log warning → manual flow for that create |
| Existing record at same name | Upsert overwrites to grey-cloud CNAME → ingress (repairs misconfig) |
| Delete: CF cleanup fails | Log warning; domain delete still succeeds |
| Delete: record content ≠ ingress host | Leave the record alone |

## Security

- Token only ever lives in the K8s Secret and gateway process env. Never
  logged, never in responses, never persisted to the DB.
- Token scoped to `Zone:Read` + `Zone.DNS:Edit` on specific zones (operator's
  choice which zones to include).
- Auto-verify does not weaken the abuse gate: automation only triggers for
  zones the *platform operator's* token controls — an arbitrary user domain on
  someone else's Cloudflare account gets `ErrZoneNotFound` and the normal
  TXT-proof path.

## Testing

- **Unit:** zone longest-suffix matching; CF client against `httptest.Server`
  stubs (create vs update branch, proxied:false in payload, delete-only-if-
  content-matches guard); createDomain fallback when client nil / zone not
  found / CF 500.
- **End-to-end (manual):** add `resume.eddisonso2.com` in the Networking tab →
  record appears grey-cloud in Cloudflare, domain verified immediately, prod
  cert issues, HTTPS 200. This simultaneously repairs the currently-broken
  orange-cloud record via the upsert.

## Effort

Small: one new ~150-line package + a guarded branch in createDomain + manifest
env + a frontend conditional. The risky surface (TLS, routing) is untouched.
