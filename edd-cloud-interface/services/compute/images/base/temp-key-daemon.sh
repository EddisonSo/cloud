#!/bin/bash
# Temp key daemon - watches for temporary SSH keys and manages their lifecycle
# Keys are written to /tmp/temp-keys/<key-id> and auto-expire after TTL

WATCH_DIR="/tmp/temp-keys"
AUTH_KEYS="/root/.ssh/authorized_keys"
KEY_TTL=${KEY_TTL:-60}  # Default 60 seconds

mkdir -p "$WATCH_DIR"
chmod 700 "$WATCH_DIR"

log() {
    echo "[temp-key-daemon] $(date '+%Y-%m-%d %H:%M:%S') $1"
}

cleanup_key() {
    local key_id="$1"
    local key_file="$WATCH_DIR/$key_id"

    if [ -f "$key_file" ]; then
        local key_content
        key_content=$(cat "$key_file")

        # Remove from authorized_keys (escape special chars for grep)
        local escaped_key
        escaped_key=$(echo "$key_content" | sed 's/[[\.*^$()+?{|]/\\&/g')
        grep -v "$escaped_key" "$AUTH_KEYS" > "$AUTH_KEYS.tmp" 2>/dev/null
        mv "$AUTH_KEYS.tmp" "$AUTH_KEYS"
        chmod 600 "$AUTH_KEYS"

        rm -f "$key_file"
        log "Removed temp key: $key_id"
    fi
}

handle_new_key() {
    local key_id="$1"
    local key_file="$WATCH_DIR/$key_id"

    # Small delay to ensure file is fully written
    sleep 0.1

    if [ -f "$key_file" ]; then
        local key_content
        key_content=$(cat "$key_file")

        # Validate it looks like an SSH key
        if echo "$key_content" | grep -qE '^(ssh-rsa|ssh-ed25519|ecdsa-sha2)'; then
            # Add to authorized_keys
            echo "$key_content" >> "$AUTH_KEYS"
            chmod 600 "$AUTH_KEYS"
            log "Added temp key: $key_id (TTL: ${KEY_TTL}s)"

            # Schedule removal after TTL
            (sleep "$KEY_TTL" && cleanup_key "$key_id") &
        else
            log "Invalid key format, ignoring: $key_id"
            rm -f "$key_file"
        fi
    fi
}

log "Starting temp key daemon (TTL: ${KEY_TTL}s)"
log "Watching: $WATCH_DIR"

# Watch for new key files
inotifywait -m -e create -e moved_to "$WATCH_DIR" --format '%f' 2>/dev/null | while read -r key_id; do
    handle_new_key "$key_id" &
done
