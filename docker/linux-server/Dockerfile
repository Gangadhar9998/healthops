FROM ubuntu:22.04

# Install SSH, nginx, python3, cron, and basic tools
RUN apt-get update -qq && \
    apt-get install -y -qq --no-install-recommends \
      openssh-server nginx python3 cron procps iproute2 curl \
      > /dev/null 2>&1 && \
    rm -rf /var/lib/apt/lists/*

# Configure SSH
RUN mkdir -p /run/sshd && \
    echo 'root:testpass123' | chpasswd && \
    sed -i 's/#PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config && \
    sed -i 's/#PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config

# Configure nginx to run in foreground (managed by entrypoint)
RUN echo "daemon off;" >> /etc/nginx/nginx.conf

# Create entrypoint that starts all services
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 22

CMD ["/entrypoint.sh"]
