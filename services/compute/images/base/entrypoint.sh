#!/bin/bash
# Copy authorized_keys from mounted secret to /root/.ssh with correct permissions
if [ -f /etc/ssh/keys/authorized_keys ]; then
    cp /etc/ssh/keys/authorized_keys /root/.ssh/authorized_keys
    chmod 600 /root/.ssh/authorized_keys
    chown root:root /root/.ssh/authorized_keys
fi

# Start sshd
exec /usr/sbin/sshd -D
