# Networking Service-Account Auth — Design

**Date:** 2026-06-13
**Status:** Approved
**Track:** 2 of 2 (depends on the scope-catalog patterns established in Track 1, but is otherwise independent and can be implemented separately)

## Goal

Let service-account tokens drive the gateway's networking API (custom domains and
Cloudflare connections), which is currently reachable only by an interactive user
session. This makes domain/DNS management automatable the same way Track 1 makes
container deploys automatable.

## Background (current state, verified in code)

- The networking endpoints live on the gateway:
  `/api/domains`, `/api/domains/{id}` and `/api/cloudflare-connections`,
  `/api/cloudflare-connections/{id}` (`edd-gateway/internal/api/server.go`).
- They are wrapped by `s.auth` (`server.go:116`), which extracts a token and
  calls `s.validator.ValidateSession(tok)` — **session JWTs only**. An `ecloud_`
  SA token is rejected outright.
- There is no `networking` (or `domains`) scope root in `validateScopes`
  (`edd-cloud-auth/internal/api/tokens.go`); `validRoots` is `{compute, storage}`.
- SA tokens are `ecloud_` + an HS256 JWT carrying a `scopes` claim, signed with
  the shared `jwtSecret`. The registry already validates them by re-fetching the
  SA's *current* scopes by `saID` (so scope edits/revocations take effect).

## Scope

**In:** a `networking` scope root with `domains` and `connections` resources;
gateway acceptance of SA tokens with scope enforcement on the networking
endpoints; frontend picker support.

**Out:** any non-networking gateway endpoints; changing the existing session-cookie
path for the dashboard (it keeps working unchanged).

## Design by component

### 1. Auth service (`edd-cloud-auth`)

- Add `networking` to `validRoots` in `validateScopes`, with resources
  `domains` and `connections` (separate resources, per the two endpoint groups),
  each allowing actions `read`, `create`, `delete`.
- Scope strings: `networking.<uid>.domains` and `networking.<uid>.connections`
  (with the usual `<uid>` ownership check and optional `.<id>` leaf for
  per-object grants, consistent with the existing model).

### 2. Gateway (`edd-gateway`)

- Extend `s.auth` to accept SA tokens in addition to session JWTs:
  1. If the bearer/cookie token has the `ecloud_` prefix, strip it and parse the
     JWT with `jwtSecret`.
  2. Resolve the SA's **current** scopes by `saID` (re-fetch, matching the
     registry's validation approach — do not trust the embedded claim, so
     revocation and scope edits are honored).
  3. Pass the resolved identity + scopes down to the handler.
- Add scope enforcement on the networking routes:
  - `/api/domains` (read on GET, create on POST), `/api/domains/{id}`
    (delete on DELETE, create on verify) → require `networking.<uid>.domains`.
  - `/api/cloudflare-connections` and `/{id}` → require
    `networking.<uid>.connections`.
- Session-authenticated dashboard requests continue to pass (a full user session
  is granted access as today); only SA tokens are newly subject to the scope
  check.

### 3. Frontend (`edd-cloud-interface/frontend`)

- Add a **Networking** group to `PermissionPicker.tsx` with the `domains` and
  `connections` resources and `read`/`create`/`delete` actions, so SA networking
  scopes can be granted from the UI.

## Data flow

```
CI/automation with SA token (networking.<uid>.domains: create)
  └─ POST cloud-api.eddisonso.com/api/domains   (Authorization: Bearer ecloud_…)
       └─ gateway s.auth: ecloud_ → parse JWT → re-fetch SA scopes by saID
            └─ enforce networking.<uid>.domains:create → handler runs
```

## Security

- SA networking access is opt-in and least-privilege: a token can be limited to
  `domains` without `connections`, or read-only, or scoped to a single object.
- Re-fetching scopes by `saID` (not trusting the JWT claim) means revoking or
  narrowing an SA takes effect immediately.
- The session path is unchanged; this only *adds* an SA path.

## Testing

- **Unit (auth):** `validateScopes` accepts `networking.<uid>.domains` and
  `.connections` with read/create/delete; rejects unknown networking resources
  and cross-user scopes.
- **Unit (gateway):** `s.auth` accepts a valid SA JWT and rejects a tampered or
  expired one; a token without the networking scope gets 403 on the networking
  routes; a session still passes; a non-networking route is unaffected.
- **Integration:** with a `networking.<uid>.domains:create` SA token, add and
  then delete a domain via the API.

## Open items / follow-ups

- Whether other gateway APIs should accept SA tokens is out of scope here; the
  `s.auth` change should be written so adding scope checks to further routes is
  straightforward.
