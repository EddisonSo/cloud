---
sidebar_position: 6
---

# Networking API

Base URL: `https://net.cloud.eddisonso.com`

The Networking API manages **custom (bring-your-own) domains**: the hostnames you
point at your containers and the Cloudflare zones used to automate their DNS. It
is served by the `edd-gateway` process on loopback and exposed to the internet
through a static route at `net.cloud.eddisonso.com`.

Two resources are managed here:

- **`/api/domains`** — the zones you own, each backed by an encrypted Cloudflare
  API token (used to automate DNS for hostnames inside that zone).
- **`/api/domain-mappings`** — individual `hostname → container port` routes,
  verified by DNS and served over on-demand HTTPS.

CORS is enabled for `cloud.eddisonso.com` and `*.cloud.eddisonso.com` origins so
the dashboard can call the API cross-origin.

:::note Historical rename
These route groups were renamed in commit `072d480`. **Owned domains** were
previously served at `/api/cloudflare-connections`, and **domain mappings** were
previously served at `/api/domains`. Older clients pinned to the old paths must
be updated.
:::

## Authentication

Every request must carry a valid JWT, either in an `Authorization: Bearer
<token>` header or in a `token` cookie. Two token kinds are accepted:

- **Session JWT** — the same interactive-session token the dashboard uses. A
  session grants full access to the caller's own resources, with no scope
  checks.
- **Service-account token** (`ecloud_…`, type `api_token`) — used for
  automation. Service-account tokens are **scope-gated** (see below).

All edd-cloud token kinds are signed with the same `JWT_SECRET`, so a valid
signature alone is not sufficient. Only interactive sessions and service-account
tokens authenticate here; **2FA-challenge tokens and registry tokens are
rejected** (registry tokens carry no `user_id` and are caught by the same
guard). A missing or invalid token returns `401 unauthorized`.

### Service-account scopes

Service-account tokens must carry a scope matching the resource and action of
the request. The scope namespace is:

```
networking.<uid>.domains
networking.<uid>.domain-mappings
```

where `<uid>` is the service account's user id. **By-id** requests (anything
under `/api/domains/<id>` or `/api/domain-mappings/<id>`, including the
`/refresh` and `/verify` sub-actions) require the most specific scope:

```
networking.<uid>.<resource>.<id>
```

Permission is evaluated with a cascade: a grant at a broader level satisfies a
more specific request. A resource-level grant (`networking.<uid>.domains`) or a
user-root grant (`networking.<uid>`) both satisfy a by-id check. The cascade
stops before the bare root — **`networking` is not assignable** and grants
nothing.

The HTTP method determines the required action:

| Method | Action | Notes |
|--------|--------|-------|
| `GET` | `read` | |
| `POST` | `create` | Applies to creates **and** the `/verify` and `/refresh` sub-actions |
| `DELETE` | `delete` | |

When a service-account token lacks the required scope, the API returns `403`
with the body:

```
forbidden: missing <scope> scope
```

(where `<scope>` is the exact scope string that was checked, including the
resource id for by-id requests).

Manage these scopes on the service account — e.g. with the CLI:

```bash
ec auth service-accounts create --name dns-bot \
  --scope networking.me.domains=read,create,delete \
  --scope networking.me.domain-mappings=read,create,delete
```

---

## Owned Domains

`/api/domains` manages the Cloudflare zones you own. Each entry stores a
Cloudflare API token (encrypted at rest) and a snapshot of the zones that token
can see. When you create a domain mapping for a hostname inside one of these
zones, the gateway uses the stored token to create the DNS record automatically.

:::note Cloudflare integration may be disabled
If the gateway is running without a configured encryption key, the Cloudflare
token integration is disabled and every `/api/domains` endpoint returns `503
cloudflare integration not configured`. Domain mappings still work via manual
DNS verification.
:::

### GET /api/domains

List your owned domains.

**Auth:** session, or `read` on `networking.<uid>.domains`

```bash
curl https://net.cloud.eddisonso.com/api/domains \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "connections": [
    {
      "id": "cfc_abc123",
      "zones": ["example.com", "example.net"],
      "created_at": "2026-06-01T12:00:00Z"
    }
  ]
}
```

Zones are lazily backfilled for entries migrated from the single-token era, so
the first list call after a migration may re-read zones from Cloudflare.

---

### POST /api/domains

Add a Cloudflare API token. The token is validated immediately by listing its
zones — if it cannot list any zones it is rejected. On success the token is
sealed (encrypted) and stored, and the visible zone names are snapshotted.

**Auth:** session, or `create` on `networking.<uid>.domains`

The token needs **Zone → Read** and **Zone → DNS → Edit** on the zones you want
covered.

```bash
curl -X POST https://net.cloud.eddisonso.com/api/domains \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"token": "cf_token_value"}'
```

**Response:** `201 Created`
```json
{
  "id": "cfc_abc123",
  "zones": ["example.com", "example.net"]
}
```

A missing or empty token returns `400 token is required`; a token that cannot
list zones returns `400 token invalid or lacks zone access`.

---

### DELETE /api/domains/\{id\}

Remove an owned domain. Existing domain mappings and any DNS records already
created are left untouched — only future DNS automation for that zone is
removed.

**Auth:** session, or `delete` on `networking.<uid>.domains.<id>`

```bash
curl -X DELETE https://net.cloud.eddisonso.com/api/domains/cfc_abc123 \
  -H "Authorization: Bearer $TOKEN"
```

**Response:** `204 No Content` (or `404 not found` if the id is not yours).

---

### POST /api/domains/\{id\}/refresh

Re-snapshot the domain's zones using its stored token. Use this after widening a
token's scope or adding a new zone to an all-zones token.

**Auth:** session, or `create` on `networking.<uid>.domains.<id>`

```bash
curl -X POST https://net.cloud.eddisonso.com/api/domains/cfc_abc123/refresh \
  -H "Authorization: Bearer $TOKEN"
```

**Response:** `200 OK`
```json
{
  "id": "cfc_abc123",
  "zones": ["example.com", "example.net", "example.org"]
}
```

If the stored token can no longer be decrypted, returns `409` (disconnect and
reconnect); if the token is no longer valid at Cloudflare, returns `502`.

---

## Domain Mappings

`/api/domain-mappings` manages individual `hostname → container port` routes. A
mapping is created in `pending` status and must be verified (via a DNS TXT
record, or automatically when the hostname is inside an owned domain) before the
gateway will serve it.

### GET /api/domain-mappings

List your domain mappings.

**Auth:** session, or `read` on `networking.<uid>.domain-mappings`

```bash
curl https://net.cloud.eddisonso.com/api/domain-mappings \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "domains": [
    {
      "id": "dom_xyz789",
      "domain": "app.example.com",
      "container_id": "ctr_123",
      "target_port": 8080,
      "status": "verified",
      "verify_name": "_edd-verify.app.example.com",
      "verify_token": "edd-verify-abc123"
    }
  ]
}
```

`status` is one of `pending`, `verified`, or `failed`. The `verify_name` /
`verify_token` fields are the TXT record to create for manual verification.

---

### POST /api/domain-mappings

Create a mapping attaching a hostname to one container port.

**Auth:** session, or `create` on `networking.<uid>.domain-mappings`

| Field | Type | Description |
|-------|------|-------------|
| `container_id` | string | Container to receive traffic (must be owned by the caller) |
| `domain` | string | Hostname to attach (e.g. `app.example.com`) |
| `target_port` | int | Container port — must be `80`, `443`, or in `8000`–`8999` |

```bash
curl -X POST https://net.cloud.eddisonso.com/api/domain-mappings \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"container_id": "ctr_123", "domain": "app.example.com", "target_port": 8080}'
```

**Response:** `201 Created` (a `domainResponse`, as in the list above). When the
hostname falls inside an owned Cloudflare zone, the gateway creates a DNS-only
CNAME to `ingress.cloud.eddisonso.com`, sets `status` to `verified`, pre-issues
the certificate, and includes `"dns_automated": true` in the response.
Otherwise the mapping starts `pending` and manual TXT verification is required.

Error cases: `400 invalid domain`, `400 port must be 80, 443, or 8000-8999`,
`404 container not found`, `403 forbidden` (container not owned by caller), or
`409 domain already in use` (a hostname can be mapped only once platform-wide).

---

### DELETE /api/domain-mappings/\{id\}

Delete a mapping. If the hostname is inside an owned Cloudflare zone, the
matching CNAME is also removed (best-effort).

**Auth:** session, or `delete` on `networking.<uid>.domain-mappings.<id>`

```bash
curl -X DELETE https://net.cloud.eddisonso.com/api/domain-mappings/dom_xyz789 \
  -H "Authorization: Bearer $TOKEN"
```

**Response:** `204 No Content` (or `404 not found` if the id is not yours).

---

### POST /api/domain-mappings/\{id\}/verify

Trigger an immediate DNS TXT check for the mapping (a background worker also
polls automatically). On a match, the status is set to `verified` and the
gateway pre-issues the certificate.

**Auth:** session, or `create` on `networking.<uid>.domain-mappings.<id>`

```bash
curl -X POST https://net.cloud.eddisonso.com/api/domain-mappings/dom_xyz789/verify \
  -H "Authorization: Bearer $TOKEN"
```

**Response:** `200 OK`
```json
{ "status": "verified" }
```

If the TXT record is not found, the response is `{"status": "pending",
"detail": "..."}`. A mapping that had expired to `failed` is reset to `pending`
on a verify attempt so the background worker resumes polling.
