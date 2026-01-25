#!/bin/bash
# Initialize NATS JetStream streams
# Run this after NATS is deployed:
#   kubectl exec -it deploy/nats -- sh /scripts/init-streams.sh
# Or from local with nats CLI:
#   nats -s nats://localhost:4222 stream add ...

set -e

NATS_URL="${NATS_URL:-nats://localhost:4222}"

echo "Connecting to NATS at $NATS_URL"

# Install nats CLI if not present (for running inside container)
if ! command -v nats &> /dev/null; then
    echo "nats CLI not found, using nats-server CLI"
    # Streams will be created programmatically by services on startup
    echo "Streams should be created by services on startup"
    exit 0
fi

# Create AUTH stream
nats stream add AUTH \
    --subjects "auth.>" \
    --retention limits \
    --max-msgs 1000000 \
    --max-bytes 1073741824 \
    --max-age 168h \
    --storage file \
    --replicas 1 \
    --discard old \
    --dupe-window 2m \
    --no-allow-rollup \
    --deny-delete \
    --deny-purge \
    || echo "AUTH stream already exists"

# Create COMPUTE stream
nats stream add COMPUTE \
    --subjects "compute.>" \
    --retention limits \
    --max-msgs 1000000 \
    --max-bytes 1073741824 \
    --max-age 168h \
    --storage file \
    --replicas 1 \
    --discard old \
    --dupe-window 2m \
    --no-allow-rollup \
    --deny-delete \
    --deny-purge \
    || echo "COMPUTE stream already exists"

# Create GATEWAY stream
nats stream add GATEWAY \
    --subjects "gateway.>" \
    --retention limits \
    --max-msgs 1000000 \
    --max-bytes 1073741824 \
    --max-age 168h \
    --storage file \
    --replicas 1 \
    --discard old \
    --dupe-window 2m \
    --no-allow-rollup \
    --deny-delete \
    --deny-purge \
    || echo "GATEWAY stream already exists"

# Create SFS stream
nats stream add SFS \
    --subjects "sfs.>" \
    --retention limits \
    --max-msgs 1000000 \
    --max-bytes 1073741824 \
    --max-age 168h \
    --storage file \
    --replicas 1 \
    --discard old \
    --dupe-window 2m \
    --no-allow-rollup \
    --deny-delete \
    --deny-purge \
    || echo "SFS stream already exists"

echo "JetStream streams initialized"
nats stream list
