FROM debian:bookworm

# Install essential packages
RUN apt-get update && apt-get install -y \
    openssh-server \
    sudo \
    curl \
    wget \
    git \
    vim \
    tmux \
    htop \
    build-essential \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Configure SSH to use keys from /etc/ssh/keys (mounted from K8s secret)
RUN mkdir /var/run/sshd && \
    mkdir -p /etc/ssh/keys && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config && \
    sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config && \
    sed -i 's/#PubkeyAuthentication yes/PubkeyAuthentication yes/' /etc/ssh/sshd_config && \
    echo "AuthorizedKeysFile /etc/ssh/keys/authorized_keys" >> /etc/ssh/sshd_config

# Create dev user with sudo
RUN useradd -m -s /bin/bash dev && \
    echo "dev ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# Expose common ports
EXPOSE 22 80 443 3000 8080

# Start SSH daemon
CMD ["/usr/sbin/sshd", "-D"]
