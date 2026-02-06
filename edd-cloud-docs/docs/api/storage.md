---
sidebar_position: 4
---

# Storage API

**Base URL:** `https://storage.cloud.eddisonso.com`

The storage service handles file uploads/downloads and namespace management, backed by a custom distributed file system (GFS).

## Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/storage/files` | Private ns | List files |
| `POST` | `/storage/upload` | Yes | Upload file |
| `GET` | `/storage/download` | Private ns | Download file |
| `DELETE` | `/storage/delete` | Yes | Delete file |
| `GET` | `/storage/:namespace/:filename` | Public ns | Direct file access |
| `GET` | `/storage/download/:namespace/:filename` | Public ns | Direct download |
| `GET` | `/storage/namespaces` | Yes | List namespaces |
| `POST` | `/storage/namespaces` | Yes | Create namespace |
| `DELETE` | `/storage/namespaces/:name` | Yes | Delete namespace |
| `PUT` | `/storage/namespaces/:name` | Yes | Update namespace |
| `GET` | `/storage/status` | No | Cluster status |

---

## Files

### List Files

```
GET /storage/files?namespace=<name>
```

**Auth:** Required for private namespaces, optional for public ones.

**Query parameters:**

| Param | Required | Description |
|-------|----------|-------------|
| `namespace` | No | Namespace to list (default: `default`) |

**Response:**

```json
[
  {
    "name": "report.pdf",
    "path": "report.pdf",
    "namespace": "default",
    "size": 1048576,
    "created_at": 1736000000,
    "modified_at": 1736000000
  }
]
```

**Example:**

```bash
curl https://storage.cloud.eddisonso.com/storage/files?namespace=default \
  -H "Authorization: Bearer $TOKEN"
```

### Upload File

```
POST /storage/upload?namespace=<name>&overwrite=<bool>
```

**Auth:** Required

Upload a file via multipart form data.

**Query parameters:**

| Param | Required | Description |
|-------|----------|-------------|
| `namespace` | No | Target namespace (default: `default`) |
| `overwrite` | No | Overwrite existing file (default: `false`) |

**Headers:**

| Header | Description |
|--------|-------------|
| `X-File-Size` | Total file size in bytes (enables progress tracking) |

**Example:**

```bash
curl -X POST "https://storage.cloud.eddisonso.com/storage/upload?namespace=default" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-File-Size: 1048576" \
  -F "file=@report.pdf"
```

**Response:**

```json
{ "status": "ok", "name": "report.pdf" }
```

Returns `409 Conflict` if the file exists and `overwrite` is not `true`.

### Download File

```
GET /storage/download?name=<filename>&namespace=<name>
```

**Auth:** Required for private namespaces.

Returns the file as an `application/octet-stream` with `Content-Disposition: attachment`.

**Example:**

```bash
curl -o report.pdf \
  "https://storage.cloud.eddisonso.com/storage/download?name=report.pdf&namespace=default" \
  -H "Authorization: Bearer $TOKEN"
```

### Delete File

```
DELETE /storage/delete?name=<filename>&namespace=<name>
```

**Auth:** Required

**Example:**

```bash
curl -X DELETE \
  "https://storage.cloud.eddisonso.com/storage/delete?name=report.pdf&namespace=default" \
  -H "Authorization: Bearer $TOKEN"
```

### Direct File Access

Files can also be accessed by path for embedding and sharing:

```
GET /storage/<namespace>/<filename>
```

Serves the file inline with the correct `Content-Type` based on extension. Accessible without auth for public/visible namespaces.

```
GET /storage/download/<namespace>/<filename>
```

Same as above but forces download via `Content-Disposition: attachment`.

---

## Namespaces

### List Namespaces

```
GET /storage/namespaces
```

Returns namespaces visible to the authenticated user. Public namespaces are shown to everyone. Private namespaces are only shown to their owner.

**Response:**

```json
[
  {
    "name": "default",
    "count": 5,
    "hidden": false,
    "visibility": 2,
    "owner_id": null
  },
  {
    "name": "my-files",
    "count": 12,
    "hidden": false,
    "visibility": 0,
    "owner_id": "XyZ123"
  }
]
```

**Visibility levels:**

| Value | Name | Description |
|-------|------|-------------|
| 0 | Private | Only owner can see and access |
| 1 | Visible | Not listed, but accessible via direct URL |
| 2 | Public | Listed and accessible to everyone |

### Create Namespace

```
POST /storage/namespaces
```

**Auth:** Required

**Request:**

```json
{
  "name": "my-files",
  "visibility": 0
}
```

Namespace names can contain letters, numbers, hyphens, underscores, and dots.

### Delete Namespace

```
DELETE /storage/namespaces/:name
```

**Auth:** Required (owner only)

Deletes the namespace and all files in it.

### Update Namespace

```
PUT /storage/namespaces/:name
```

**Auth:** Required (owner only)

**Request:**

```json
{ "visibility": 1 }
```

---

## Cluster Status

```
GET /storage/status
```

Returns GFS cluster health.

**Response:**

```json
{
  "chunkserver_count": 3,
  "total_servers": 3
}
```

---

## Health Check

```
GET /healthz
```

Returns `200 OK` with body `ok`.
