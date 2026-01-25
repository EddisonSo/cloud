FROM buildpack-deps:bookworm

# Install additional packages (buildpack-deps already has build-essential, curl, wget, git, ca-certificates)
RUN apt-get update && apt-get install -y \
    openssh-server \
    sudo \
    vim \
    tmux \
    htop \
    inotify-tools \
    && rm -rf /var/lib/apt/lists/*

# Configure SSH - keys mounted at /etc/ssh/keys, copied to /root/.ssh on startup
RUN mkdir /var/run/sshd && \
    mkdir -p /root/.ssh && \
    chmod 700 /root/.ssh && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config && \
    sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config && \
    sed -i 's/#PubkeyAuthentication yes/PubkeyAuthentication yes/' /etc/ssh/sshd_config

# Add temp key daemon for cloud terminal support
COPY temp-key-daemon.sh /usr/local/bin/temp-key-daemon.sh
RUN chmod +x /usr/local/bin/temp-key-daemon.sh

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Create dev user with sudo
RUN useradd -m -s /bin/bash dev && \
    echo "dev ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# Configure tmux with mouse support
RUN echo "set -g mouse on" > /etc/tmux.conf

# Expose common ports
EXPOSE 22 80 443 3000 8080

# Start via entrypoint (copies keys and starts sshd)
CMD ["/entrypoint.sh"]
