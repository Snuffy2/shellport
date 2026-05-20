# ShellPort is built as a static Go binary, but the production build also needs
# Node because `npm run build` runs the frontend toolchain before building the
# Go binary with the generated assets embedded.
#
# The Docker build uses a separate Node dependency stage and then a Go builder
# stage. Node is copied from the dependency stage so frontend dependencies stay
# cacheable without installing Node through apt inside the Go image.
#
# The runtime stage is Alpine and contains only the compiled `/shellport` binary
# and a small entrypoint wrapper for optional Docker TLS environment variables.
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

# Build the ET client
FROM alpine:3.23 AS et-builder
ARG ET_COMMIT=7f566c0f3504e32554d98e71e573976255af91fb
WORKDIR /src
RUN set -ex && \
    apk add --no-cache build-base cmake git pkgconf openssl-dev zlib-dev libsodium-dev protobuf-dev protobuf libunwind-dev libutempter-dev && \
    git clone --recurse-submodules --depth 1 --branch et-v6.2.11 https://github.com/MisterTea/EternalTerminal.git /src/EternalTerminal && \
    test "$(git -C /src/EternalTerminal rev-parse HEAD)" = "$ET_COMMIT" && \
    cmake -S /src/EternalTerminal -B /src/EternalTerminal/build -DCMAKE_BUILD_TYPE=Release -DDISABLE_VCPKG=ON -DBUILD_TESTING=OFF -DDISABLE_SENTRY=ON -DDISABLE_TELEMETRY=ON && \
    cmake --build /src/EternalTerminal/build --target et --parallel "$(nproc)" && \
    install -Dm755 /src/EternalTerminal/build/et /out/et

# Build the final image for running
FROM alpine:3.23
ARG SHELLPORT_SOURCE_URL=https://github.com/Snuffy2/shellport
ARG ET_COMMIT=7f566c0f3504e32554d98e71e573976255af91fb
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
COPY --from=et-builder /out/et /usr/local/bin/et
COPY --from=builder /shellport /
COPY docker-entrypoint.sh /shellport.sh
RUN set -ex && \
    printf '%s\n' \
        'ShellPort source code' \
        '' \
        "The corresponding source for this image is available at:" \
        "$SHELLPORT_SOURCE_URL" \
        '' \
        'Bundled Eternal Terminal (ET) client' \
        '' \
        'The ET client binary in this image is built from:' \
        "https://github.com/MisterTea/EternalTerminal/tree/${ET_COMMIT}" \
        'License: Apache-2.0' \
        > /SOURCE.md && \
    apk add --no-cache abseil-cpp-vlog-config-internal libgcc libprotobuf libstdc++ libunwind libsodium openssh-client openssl tzdata zlib && \
    adduser -D shellport && \
    chmod +x /usr/local/bin/et && \
    chmod +x /shellport && \
    chmod +x /shellport.sh
USER shellport
EXPOSE 8182
ENTRYPOINT [ "/shellport.sh" ]
CMD []
