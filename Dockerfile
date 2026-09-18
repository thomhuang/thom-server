# syntax=docker/dockerfile:1

FROM golang:1.25 AS build

# Set destination for COPY
WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

# Install the vulnerability scanner in its own layer so a source change does
# not force a re-download. Pinned to the newest release that still supports the
# Go toolchain in this base image (v1.8.0 requires Go 1.26).
RUN go install golang.org/x/vuln/cmd/govulncheck@v1.7.0

# copy the source code, .dockerignore handling irrelevant files
COPY . ./

# Fail the image build when a known vulnerability is reachable from this code.
# govulncheck only exits non-zero for vulnerabilities our code actually calls,
# so unrelated advisories in the dependency tree do not block deploys.
RUN govulncheck ./...

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /web-server ./cmd/server

# Runtime image: the server talks to Cloudflare D1 over HTTPS, so it only
# needs CA certificates alongside the statically-built binary.
FROM debian:bookworm-slim

# The server only accepts connections on 4000 and reaches Cloudflare's APIs
# over HTTPS, so it has no reason to run as root. 10001 is an unprivileged
# uid/gid that needs no /etc/passwd entry, and /app is owned by it so the
# working directory is writable.
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates \
	&& rm -rf /var/lib/apt/lists/* \
	&& mkdir -p /app/internal \
	&& chown -R 10001:10001 /app

COPY --from=build --chown=10001:10001 /web-server /web-server

WORKDIR /app

# server port
EXPOSE 4000

USER 10001:10001

# Run
CMD ["/web-server"]
