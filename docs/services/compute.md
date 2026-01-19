---
sidebar_position: 3
---

# Compute Service

The Compute Service manages containers running on the Kubernetes cluster. It provides a simplified interface for container lifecycle management.

## Features

- **Container Management**: Create, start, stop, delete containers
- **SSH Access**: Enable SSH access to containers
- **Port Forwarding**: Expose container ports via ingress rules
- **Real-time Updates**: WebSocket-based status updates

## API Endpoints

### Containers

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/compute/containers` | List user's containers |
| POST | `/compute/containers` | Create container |
| DELETE | `/compute/containers/:id` | Delete container |
| POST | `/compute/containers/:id/start` | Start container |
| POST | `/compute/containers/:id/stop` | Stop container |

### SSH Keys

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/compute/ssh-keys` | List SSH keys |
| POST | `/compute/ssh-keys` | Add SSH key |
| DELETE | `/compute/ssh-keys/:id` | Remove SSH key |

### Container Access

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/compute/containers/:id/ssh` | Get SSH status |
| PUT | `/compute/containers/:id/ssh` | Toggle SSH access |
| GET | `/compute/containers/:id/ingress` | List ingress rules |
| POST | `/compute/containers/:id/ingress` | Add ingress rule |
| DELETE | `/compute/containers/:id/ingress/:port` | Remove ingress rule |

### WebSocket

| Endpoint | Description |
|----------|-------------|
| `/compute/ws` | Real-time container status updates |

## Container Lifecycle

```
┌─────────┐    create    ┌─────────┐    start    ┌─────────┐
│ (none)  │ ──────────→  │ stopped │ ──────────→ │ running │
└─────────┘              └─────────┘              └─────────┘
                              ↑         stop          │
                              └───────────────────────┘
```

## WebSocket Updates

Real-time container status via WebSocket:

```javascript
const ws = new WebSocket('wss://compute.cloud.eddisonso.com/compute/ws');

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);

  if (msg.type === 'containers') {
    // Full container list
    console.log(msg.data);
  } else if (msg.type === 'container_status') {
    // Status update for single container
    console.log(msg.data.container_id, msg.data.status);
  }
};
```

## SSH Access

When SSH is enabled for a container:

1. Container gets an SSH server sidecar
2. User's SSH keys are injected
3. Access via: `ssh -p 2222 user@cloud.eddisonso.com`

## Ingress Rules

Expose container ports to the internet:

```json
{
  "port": 443,
  "target_port": 8080
}
```

This creates an ingress rule routing `container.eddisonso.com:443` to the container's port 8080.

## Database Schema

```sql
CREATE TABLE containers (
    id TEXT PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    name TEXT NOT NULL,
    image TEXT NOT NULL,
    status TEXT DEFAULT 'stopped',
    ssh_enabled BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE ingress_rules (
    id SERIAL PRIMARY KEY,
    container_id TEXT REFERENCES containers(id),
    port INTEGER NOT NULL,
    target_port INTEGER NOT NULL
);

CREATE TABLE ssh_keys (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    name TEXT NOT NULL,
    public_key TEXT NOT NULL
);
```
