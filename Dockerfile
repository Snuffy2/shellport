# ShellPort is built as a static Go binary, but the production build also needs
# Node because `npm run build` runs the frontend toolchain before building the
# Go binary with the generated assets embedded.
#
# The Docker build uses a separate Node dependency stage and then a Go builder
# stage. Node is copied from the dependency stage so frontend dependencies stay
# cacheable without installing Node through apt inside the Go image.
#
# The runtime stage is Debian Trixie slim and installs the packaged ET client
# from Eternal Terminal's Debian repository instead of compiling ET from source.
# Source availability is provided by an in-image source notice, the app's source
# link, and the OCI source metadata label rather than by copying source files
# into the image. Release builds pass an immutable commit archive URL as
# SHELLPORT_SOURCE_URL.

# Build the frontend dependencies
FROM node:24-trixie AS frontend-deps
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci

# Build the application binary
FROM golang:1.26-trixie AS builder
WORKDIR /src
ARG SHELLPORT_VERSION=dev
ARG SHELLPORT_SOURCE_URL=https://github.com/Snuffy2/shellport
COPY go.mod go.sum ./
RUN go mod download
COPY --from=frontend-deps /usr/local/bin/node /usr/local/bin/node
COPY --from=frontend-deps /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN set -ex && \
    ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx
COPY --from=frontend-deps /src/node_modules ./node_modules
COPY package.json package-lock.json ./
COPY . .
RUN set -ex && \
    SHELLPORT_SOURCE_URL="$SHELLPORT_SOURCE_URL" SHELLPORT_VERSION="$SHELLPORT_VERSION" npm run build && \
    mv ./shellport /

# Build the final image for running
FROM debian:trixie-slim
ARG SHELLPORT_SOURCE_URL=https://github.com/Snuffy2/shellport
LABEL org.opencontainers.image.licenses="AGPL-3.0-only AND Apache-2.0"
ENV SHELLPORT_DIALTIMEOUT=10 \
    SHELLPORT_DEBUG= \
    SHELLPORT_HOOKTIMEOUT=30 \
    SHELLPORT_LISTENINTERFACE=0.0.0.0 \
    SHELLPORT_LISTENPORT=8182 \
    SHELLPORT_INITIALTIMEOUT=0 \
    SHELLPORT_READTIMEOUT=0 \
    SHELLPORT_WRITETIMEOUT=0 \
    SHELLPORT_HEARTBEATTIMEOUT=0 \
    SHELLPORT_READDELAY=0 \
    SHELLPORT_WRITEDELAY=0
COPY --from=builder /shellport /
COPY docker-entrypoint.sh /shellport.sh
RUN set -ex && \
    apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl && \
    install -d -m 0755 /etc/apt/keyrings && \
    curl -fsSL https://github.com/MisterTea/debian-et/raw/master/et.gpg -o /etc/apt/keyrings/et.gpg && \
    printf '%s\n' \
        'deb [signed-by=/etc/apt/keyrings/et.gpg] https://mistertea.github.io/debian-et/debian-source/ trixie main' \
        > /etc/apt/sources.list.d/et.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends et openssh-client tzdata && \
    printf '%s\n' \
        'ShellPort source code' \
        '' \
        "The corresponding source for this image is available at:" \
        "$SHELLPORT_SOURCE_URL" \
        '' \
        'Bundled Eternal Terminal (ET) client' \
        '' \
        'The ET client binary in this image is installed from:' \
        'https://mistertea.github.io/debian-et/debian-source/ trixie main' \
        'Upstream source:' \
        'https://github.com/MisterTea/EternalTerminal' \
        'License: Apache-2.0' \
        > /SOURCE.md && \
    apt-get purge -y --auto-remove curl && \
    rm -rf /var/lib/apt/lists/* && \
    useradd --system --create-home --shell /usr/sbin/nologin shellport && \
    chmod +x /shellport && \
    chmod +x /shellport.sh
USER shellport
EXPOSE 8182
ENTRYPOINT [ "/shellport.sh" ]
CMD []
