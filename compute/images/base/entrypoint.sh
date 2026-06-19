#!/bin/bash
# Container entrypoint - sets up SSH keys and starts services

# Create .ssh directory with correct permissions
mkdir -p /root/.ssh
chmod 700 /root/.ssh
chown root:root /root/.ssh

# Copy authorized_keys from mounted secret to /root/.ssh with correct permissions
if [ -f /etc/ssh/keys/authorized_keys ]; then
    cp /etc/ssh/keys/authorized_keys /root/.ssh/authorized_keys
    chmod 600 /root/.ssh/authorized_keys
    chown root:root /root/.ssh/authorized_keys
fi

# Ensure authorized_keys exists (for temp keys to append to)
touch /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys

# Start temp key daemon in background
/usr/local/bin/temp-key-daemon.sh &

# Start sshd in foreground
exec /usr/sbin/sshd -D
