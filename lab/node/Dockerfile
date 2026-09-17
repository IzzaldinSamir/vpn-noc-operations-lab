FROM alpine:3.22

RUN apk add --no-cache \
      bash \
      bind-tools \
      busybox-extras \
      curl \
      dnsmasq \
      iproute2 \
      iproute2-tc \
      iptables \
      iputils \
      tcpdump \
      wireguard-tools

COPY entrypoint.sh /usr/local/bin/lab-node
RUN chmod 0755 /usr/local/bin/lab-node

ENTRYPOINT ["/usr/local/bin/lab-node"]
