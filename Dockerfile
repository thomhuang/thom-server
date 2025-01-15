# syntax=docker/dockerfile:1

FROM golang:1.22

# Set destination for COPY
WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

# copy the source code, .dockerignore handling irrelevant files
COPY . ./

# Build
RUN CGO_ENABLED=1 GOOS=linux go build -o /web-server ./cmd/server

# server port
EXPOSE 4000

# Run
CMD ["/web-server"]