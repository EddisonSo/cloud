---
sidebar_position: 4
---

# Storage API

Base URL: `https://storage.cloud.eddisonso.com`

## Reference

### Visibility Levels

| Value | Name | Behavior |
|-------|------|----------|
| `0` | Private | Only the owner can see and access |
| `1` | Visible | Only the owner sees in listings, but accessible via direct URL |
| `2` | Public | Everyone can see and access |

---

## Namespaces

### GET /storage/namespaces

List namespaces. Private namespaces are only shown to the owner.

**Auth:** Session / API token (optional — public namespaces are always listed)
**Token Scope:** `storage.<uid>.namespaces` with `read`

**Example request:**
```bash
curl https://storage.cloud.eddisonso.com/storage/namespaces \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
[
  {
    "name": "my-files",
    "count": 12,
    "visibility": 2,
    "owner_id": "abc123"
  }
]
```

---

### POST /storage/namespaces

Create a new namespace.

**Auth:** Session / API token
**Token Scope:** `storage.<uid>.namespaces` with `create`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| name | string | body | Yes | Namespace name (alphanumeric, hyphens, underscores, dots) |
| visibility | int | body | No | `0`, `1`, or `2` (default `2`) |

**Example request:**
```bash
curl -X POST https://storage.cloud.eddisonso.com/storage/namespaces \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"name": "my-files", "visibility": 0}'
```

**Response:**
```json
{
  "name": "my-files",
  "count": 0,
  "visibility": 0,
  "owner_id": "abc123"
}
```

---

### DELETE /storage/namespaces/:name

Delete a namespace and all its files. Triggers a "Namespace Deleted" notification.

**Auth:** Session / API token
**Token Scope:** `storage.<uid>.namespaces.<name>` with `delete`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| name | string | path | Yes | Namespace name |

**Example request:**
```bash
curl -X DELETE https://storage.cloud.eddisonso.com/storage/namespaces/my-files \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "status": "ok"
}
```

---

### PUT /storage/namespaces/:name

Update namespace visibility.

**Auth:** Session / API token
**Token Scope:** `storage.<uid>.namespaces.<name>` with `update`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| name | string | path | Yes | Namespace name |
| visibility | int | body | Yes | `0`, `1`, or `2` |

**Example request:**
```bash
curl -X PUT https://storage.cloud.eddisonso.com/storage/namespaces/my-files \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"visibility": 1}'
```

**Response:**
```json
{
  "name": "my-files",
  "visibility": 1
}
```

---

## Files

### GET /storage/files

List files in a namespace.

**Auth:** Required for private namespaces, optional for public/visible
**Token Scope:** `storage.<uid>.files.<namespace>` with `read`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| namespace | string | query | No | Namespace name (default `"default"`) |

**Example request:**
```bash
curl "https://storage.cloud.eddisonso.com/storage/files?namespace=my-files" \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
[
  {
    "name": "report.pdf",
    "path": "/my-files/report.pdf",
    "namespace": "my-files",
    "size": 204800,
    "created_at": 1712224000,
    "modified_at": 1712224000
  }
]
```

---

### POST /storage/:namespace/:filename

Upload a file using multipart form data or raw body.

**Auth:** Session / API token
**Token Scope:** `storage.<uid>.files.<namespace>` with `create`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| namespace | string | path | Yes | Target namespace |
| filename | string | path | Yes | File name |
| overwrite | string | query | No | `"true"` to overwrite existing files |
| file | file | form | No | File to upload (multipart/form-data) |

**Example request (multipart):**
```bash
curl -X POST "https://storage.cloud.eddisonso.com/storage/my-files/report.pdf" \
  -H "Authorization: Bearer eyJhbGci..." \
  -F "file=@report.pdf"
```

**Example request (raw body):**
```bash
curl -X POST "https://storage.cloud.eddisonso.com/storage/my-files/report.pdf" \
  -H "Authorization: Bearer eyJhbGci..." \
  --data-binary "@report.pdf"
```

**Response:**
```json
{
  "status": "ok",
  "name": "report.pdf",
  "namespace": "my-files"
}
```

Returns `409` if the file already exists and `overwrite` is not set. Triggers a "File Uploaded" notification.

**Note:** The legacy endpoint `POST /storage/upload?namespace=...` is still supported for backward compatibility.

---

### POST /storage/upload (legacy)

Legacy upload endpoint using query parameters. **Prefer the REST-style `POST /storage/:namespace/:filename` endpoint above.**

**Auth:** Session / API token
**Token Scope:** `storage.<uid>.files.<namespace>` with `create`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| namespace | string | query | No | Target namespace (default `"default"`) |
| overwrite | string | query | No | `"true"` to overwrite existing files |
| file | file | form | Yes | File to upload |

**Example request:**
```bash
curl -X POST "https://storage.cloud.eddisonso.com/storage/upload?namespace=my-files" \
  -H "Authorization: Bearer eyJhbGci..." \
  -F "file=@report.pdf"
```

**Response:**
```json
{
  "status": "ok",
  "name": "report.pdf"
}
```

Returns `409` if the file already exists and `overwrite` is not set. Triggers a "File Uploaded" notification.

---

### GET /storage/download

Download a file (forced attachment download).

**Auth:** Required for private namespaces, optional for public/visible
**Token Scope:** `storage.<uid>.files.<namespace>` with `read`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| name | string | query | Yes | Filename |
| namespace | string | query | No | Namespace name (default `"default"`) |
| token | string | query | No | JWT for shareable links |

**Example request:**
```bash
curl -o report.pdf \
  "https://storage.cloud.eddisonso.com/storage/download?namespace=my-files&name=report.pdf" \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:** Binary file with `Content-Disposition: attachment`.

---

### DELETE /storage/:namespace/:filename

Delete a file. Triggers a "File Deleted" notification.

**Auth:** Session / API token
**Token Scope:** `storage.<uid>.files.<namespace>` with `delete`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| namespace | string | path | Yes | Namespace name |
| filename | string | path | Yes | File name to delete |

**Example request:**
```bash
curl -X DELETE \
  "https://storage.cloud.eddisonso.com/storage/my-files/report.pdf" \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "status": "ok",
  "name": "report.pdf"
}
```

**Note:** The legacy endpoint `DELETE /storage/delete?namespace=...&name=...` is still supported for backward compatibility.

---

### DELETE /storage/delete (legacy)

Legacy delete endpoint using query parameters. **Prefer the REST-style `DELETE /storage/:namespace/:filename` endpoint above.**

**Auth:** Session / API token
**Token Scope:** `storage.<uid>.files.<namespace>` with `delete`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| name | string | query | Yes | Filename to delete |
| namespace | string | query | No | Namespace name (default `"default"`) |

**Example request:**
```bash
curl -X DELETE \
  "https://storage.cloud.eddisonso.com/storage/delete?namespace=my-files&name=report.pdf" \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "status": "ok",
  "name": "report.pdf"
}
```

---

## Public Access

### GET /storage/:namespace/:filename

Serve a file inline (auto-detected content type). Available for public and visible namespaces without authentication.

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| namespace | string | path | Yes | Namespace name |
| filename | string | path | Yes | File path |
| token | string | query | No | JWT for shareable links |

**Example request:**
```bash
curl https://storage.cloud.eddisonso.com/storage/my-files/image.png -o image.png
```

**Response:** Binary file with auto-detected `Content-Type` (e.g. `image/png`, `text/html`).

---

### GET /storage/download/:namespace/:filename

Force-download a file from a public or visible namespace.

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| namespace | string | path | Yes | Namespace name |
| filename | string | path | Yes | File path |
| token | string | query | No | JWT for shareable links |

**Example request:**
```bash
curl -o image.png \
  https://storage.cloud.eddisonso.com/storage/download/my-files/image.png
```

**Response:** Binary file with `Content-Disposition: attachment`.

---

## Status

### GET /storage/status

Get cluster storage health.

**Auth:** None

**Example request:**
```bash
curl https://storage.cloud.eddisonso.com/storage/status
```

**Response:**
```json
{
  "chunkserver_count": 3,
  "total_servers": 3
}
```
