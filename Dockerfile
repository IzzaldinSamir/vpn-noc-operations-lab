FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vpn-exporter ./cmd/vpn-exporter \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/incident-webhook ./cmd/incident-webhook

FROM alpine:3.22 AS vpn-exporter
RUN apk add --no-cache wireguard-tools ca-certificates \
    && addgroup -S exporter \
    && adduser -S -G exporter exporter
COPY --from=build /out/vpn-exporter /usr/local/bin/vpn-exporter
EXPOSE 9586
ENTRYPOINT ["/usr/local/bin/vpn-exporter"]

FROM scratch AS incident-webhook
COPY --from=build /out/incident-webhook /incident-webhook
EXPOSE 8081
ENTRYPOINT ["/incident-webhook"]
