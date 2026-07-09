FROM ubuntu:24.04

ARG TARGETARCH
ARG NODE_VERSION=24.18.0

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        sudo \
        tar \
        unzip \
        xz-utils \
    && case "$TARGETARCH" in \
        amd64) node_arch="x64" ;; \
        arm64) node_arch="arm64" ;; \
        *) echo "Unsupported runner architecture: $TARGETARCH" >&2; exit 2 ;; \
    esac \
    && curl --fail --show-error --location \
        "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${node_arch}.tar.xz" \
        --output /tmp/node.tar.xz \
    && tar -xJf /tmp/node.tar.xz --strip-components=1 -C /usr/local \
    && rm -f /tmp/node.tar.xz \
    && rm -rf /var/lib/apt/lists/* \
    && node --version
