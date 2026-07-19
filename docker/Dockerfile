# syntax=docker/dockerfile:1.7

ARG QBITTORRENT_IMAGE=lscr.io/linuxserver/qbittorrent:latest

FROM --platform=$BUILDPLATFORM node:22-alpine AS webui-build

WORKDIR /src/webui
RUN npm install --global pnpm@11.10.0
COPY webui/package.json webui/pnpm-lock.yaml webui/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY webui/ ./
RUN pnpm run build

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS nexusbridge-build

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=webui-build /src/webui/dist /src/webui/dist
RUN CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" \
    go build -tags production -trimpath -ldflags="-s -w" \
    -o /out/nexusbridge ./cmd/nexusbridge

FROM --platform=$BUILDPLATFORM alpine:3.23 AS mihomo-download

ARG TARGETARCH
ARG MIHOMO_VERSION=v1.19.29
COPY docker/root/defaults/mihomo/config.yaml /tmp/mihomo-config.yaml
RUN apk add --no-cache ca-certificates curl gzip unzip && \
    case "$TARGETARCH" in \
      amd64|arm64) mihomo_arch="$TARGETARCH" ;; \
      *) echo "unsupported mihomo architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac && \
    curl --fail --show-error --silent --location \
      --connect-timeout 20 --max-time 300 \
      --retry 5 --retry-delay 2 --retry-all-errors \
      "https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VERSION}/mihomo-linux-${mihomo_arch}-${MIHOMO_VERSION}.gz" \
      --output /tmp/mihomo.gz && \
    gzip -dc /tmp/mihomo.gz > /out-mihomo && \
    chmod 0755 /out-mihomo && \
    config_url() { \
      awk -v key="$1:" '$1 == key { print $2; exit }' /tmp/mihomo-config.yaml; \
    } && \
    mkdir -p /out-mihomo-defaults/ui /tmp/mihomo-ui && \
    curl --fail --show-error --silent --location \
      --connect-timeout 20 --max-time 300 \
      --retry 5 --retry-delay 2 --retry-all-errors \
      "$(config_url external-ui-url)" \
      --output /tmp/mihomo-ui.zip && \
    unzip -q /tmp/mihomo-ui.zip -d /tmp/mihomo-ui && \
    ui_index="$(find /tmp/mihomo-ui -type f -name index.html -print -quit)" && \
    test -n "$ui_index" && \
    cp -a "$(dirname "$ui_index")"/. /out-mihomo-defaults/ui/ && \
    curl --fail --show-error --silent --location \
      --connect-timeout 20 --max-time 300 \
      --retry 5 --retry-delay 2 --retry-all-errors \
      "$(config_url geoip)" \
      --output /out-mihomo-defaults/GeoIP.dat && \
    curl --fail --show-error --silent --location \
      --connect-timeout 20 --max-time 300 \
      --retry 5 --retry-delay 2 --retry-all-errors \
      "$(config_url geosite)" \
      --output /out-mihomo-defaults/GeoSite.dat && \
    curl --fail --show-error --silent --location \
      --connect-timeout 20 --max-time 300 \
      --retry 5 --retry-delay 2 --retry-all-errors \
      "$(config_url mmdb)" \
      --output /out-mihomo-defaults/country.mmdb && \
    curl --fail --show-error --silent --location \
      --connect-timeout 20 --max-time 300 \
      --retry 5 --retry-delay 2 --retry-all-errors \
      "$(config_url asn)" \
      --output /out-mihomo-defaults/ASN.mmdb && \
    test -s /out-mihomo-defaults/ui/index.html && \
    test -s /out-mihomo-defaults/GeoIP.dat && \
    test -s /out-mihomo-defaults/GeoSite.dat && \
    test -s /out-mihomo-defaults/country.mmdb && \
    test -s /out-mihomo-defaults/ASN.mmdb

FROM ${QBITTORRENT_IMAGE}

ARG GITHUB_REPOSITORY=Huffer342-WSH/NexusBridge
ARG MIHOMO_VERSION=v1.19.29
LABEL org.opencontainers.image.title="NexusBridge all-in-one" \
      org.opencontainers.image.description="NexusBridge, qBittorrent-nox and mihomo in one multi-architecture image" \
      org.opencontainers.image.source="https://github.com/${GITHUB_REPOSITORY}" \
      org.opencontainers.image.mihomo.version="$MIHOMO_VERSION"

USER root
RUN apk add --no-cache ca-certificates nano && update-ca-certificates

COPY --from=nexusbridge-build /out/nexusbridge /app/nexusbridge
COPY --from=mihomo-download /out-mihomo /app/mihomo
COPY --from=mihomo-download /out-mihomo-defaults/ /defaults/mihomo/
COPY docker/root/ /

RUN chmod 0755 \
      /app/nexusbridge \
      /app/mihomo \
      /etc/s6-overlay/s6-rc.d/svc-nexusbridge/run \
      /etc/s6-overlay/s6-rc.d/svc-mihomo/run

ENV NEXUSBRIDGE_DATA_DIR=/config/nexusbridge \
    NEXUSBRIDGE_MIHOMO_CONFIG_DIR=/config/mihomo

EXPOSE 16881/tcp 16881/udp 7890/tcp 8080/tcp 8090/tcp 9090/tcp
VOLUME ["/config", "/downloads"]
