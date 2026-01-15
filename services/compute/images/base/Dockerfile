FROM debian:bookworm

# Configure apt sources for more packages
RUN sed -i 's/^deb /deb [signed-by=\/usr\/share\/keyrings\/debian-archive-keyring.gpg] /' /etc/apt/sources.list && \
    sed -i 's/bookworm main/bookworm main contrib non-free non-free-firmware/' /etc/apt/sources.list

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

# Configure SSH - keys mounted at /etc/ssh/keys, copied to /root/.ssh on startup
RUN mkdir /var/run/sshd && \
    mkdir -p /root/.ssh && \
    chmod 700 /root/.ssh && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config && \
    sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config && \
    sed -i 's/#PubkeyAuthentication yes/PubkeyAuthentication yes/' /etc/ssh/sshd_config

# Copy authorized_keys from mounted secret and fix permissions before starting sshd
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Create dev user with sudo
RUN useradd -m -s /bin/bash dev && \
    echo "dev ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# Expose common ports
EXPOSE 22 80 443 3000 8080

# Start via entrypoint (copies keys and starts sshd)
CMD ["/entrypoint.sh"]
