# Download Daily Logs — Design

**Date:** 2026-06-20
**Status:** Approved (design)
**Services touched:** `log-service` (new endpoint), `edd-cloud-interface` frontend (Logs page UI)

## Problem

The dashboard's Health → Logs view streams live logs over `/ws/logs`, but there is
no way to obtain a persisted record of a day's logs for incident review or archival.
Logs are already persisted to GFS by `log-service` as one file per source per day:

```
/<YYYY-MM-DD>/<source>.jsonl      e.g. /2026-06-20/edd-gateway.jsonl
```

We want an admin-triggered "download this day's logs" action that returns a single
archive covering all sources for the chosen date.

## Goals

- Admin can pick a date and download **all sources' logs for that day** merged into
  one chronological record.
- Archive contains **both** a human-readable `.log` and the raw structured `.jsonl`.
- Reuse existing auth (admin-only, same as the live stream) and the established
  cross-origin download pattern (hidden iframe).

## Non-goals (YAGNI)

- Date ranges, per-source downloads, or filtered downloads (current Source/Level
  filters do not apply to the download).
- Streaming/pagination of arbitrarily huge days — a day's logs fit comfortably in
  memory for this deployment.
- Changing how logs are persisted to GFS.

## User experience

On the Logs page toolbar (next to the existing **CLEAR** control):

- A native date input, **defaulting to today**, allowing selection of any past date.
- A **Download** button. Clicking it downloads `edd-cloud-logs-<date>.zip`.
- If the day has no logs, the backend returns `404` and the UI surfaces a
  "no logs for that date" status message (no file is downloaded).

The download is initiated via a hidden `<iframe>` whose `src` is the backend URL with
the session token in the query string, matching the existing storage-download pattern
(the `download` attribute is ignored cross-origin, and an iframe keeps navigation off
the SPA while `Content-Disposition: attachment` triggers the browser download manager).

## Backend design (log-service)

New HTTP endpoint:

```
GET /logs/download?date=YYYY-MM-DD
```

**Auth:** wrapped in the existing `requireAdminForLogs` middleware (same as `/ws/logs`):
missing/invalid JWT → 401, valid non-admin → 403. Token is read via the existing
`getTokenFromRequest` (Authorization header, `?token=` query param, or `token` cookie),
so the browser iframe passes `?token=<session token>`. CORS via the existing
`corsMiddleware` so the cross-origin request from `cloud.eddisonso.com` is allowed.

**Handler flow:**

1. Parse and validate `date` (must be `YYYY-MM-DD`; reject malformed input with 400).
2. List GFS files under the `/<date>/` prefix via the GFS SDK (the same list-by-prefix
   capability `sfs` uses). Each file is `<source>.jsonl`.
3. If no files / no entries → `404`.
4. Read each source file, parse entries (JSONL), and merge into one slice.
5. Sort merged entries by timestamp ascending.
6. Build a zip in memory containing two members:
   - `edd-cloud-logs-<date>.log` — one formatted line per entry:
     `2006-01-02 15:04:05  <LEVEL>  <source>  <message>` (matching the UI rendering).
   - `edd-cloud-logs-<date>.jsonl` — the merged, sorted raw JSONL.
7. Respond with `Content-Type: application/zip` and
   `Content-Disposition: attachment; filename="edd-cloud-logs-<date>.zip"`, writing the
   zip bytes to the response.

**Error handling:**

- Malformed `date` → 400.
- No logs for the date → 404 (UI shows "no logs for that date").
- GFS read failure (e.g. a chunkserver down) → 502, with the error logged at WARN. A
  partial read where *some* sources are unreadable should still return what it can and
  log a WARN naming the missing source(s), rather than failing the whole download.

## Components & boundaries

- **`log-service` download handler** — new function in `log-service` HTTP layer. Input:
  validated date + admin claims. Output: zip bytes or an HTTP error. Depends on the GFS
  SDK (list + read) and the JSONL log entry type already used by the persistence worker.
- **`log-service` log formatting helper** — pure function: `(entry) -> "ts LEVEL source msg"`.
  Independently testable; mirrors the frontend's `formatTimestamp`/level mapping.
- **Frontend Logs toolbar** — date input + Download button in the existing Logs
  component; calls a small `downloadDailyLogs(date)` that builds the URL (health base +
  `/logs/download?date=...&token=...`) and triggers the hidden-iframe download. No new
  state beyond the selected date and a status/error string.

## Testing

- **Backend unit tests:** date validation (valid/malformed); merge+sort across multiple
  source files with interleaved timestamps; empty-day → 404; the `.log` formatting
  helper; partial-read behaviour (one source unreadable → WARN + remaining sources still
  included). Zip contains exactly the two expected members with correct names.
- **Manual/integration:** download a real day from the dashboard; verify the zip opens,
  the `.log` is human-readable and chronologically ordered, and the `.jsonl` round-trips
  through a JSON parser. Verify non-admin gets 403 and tokenless gets 401.

## Known caveat

GFS persistence is currently degraded (the `s1` chunkserver is down — `failed to persist
logs … no route to host`). Recent daily files may therefore be incomplete until `s1` is
restored. This is an operational issue separate from this feature; the design is correct
regardless, and the partial-read WARN behaviour above makes incomplete data visible
rather than silently failing.
```
