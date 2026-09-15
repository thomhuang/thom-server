# syntax=docker/dockerfile:1

FROM golang:1.22 AS build

# Set destination for COPY
WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

# copy the source code, .dockerignore handling irrelevant files
COPY . ./

# Build
RUN CGO_ENABLED=1 GOOS=linux go build -o /web-server ./cmd/server

# Runtime image: the server talks to Cloudflare D1 over HTTPS, so it only
# needs CA certificates alongside the statically-built binary.
FROM debian:bookworm-slim

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

COPY --from=build /web-server /web-server

# server port
EXPOSE 4000

# Run
CMD ["/web-server"]
