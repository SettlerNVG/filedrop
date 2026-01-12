# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git for go mod (some deps need it)
RUN apk add --no-cache git

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build all binaries
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.Version=$(git describe --tags --always 2>/dev/null || echo dev)" -o /bin/relay ./cmd/relay
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/filedrop ./cmd/client
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/filedrop-tui ./cmd/tui

# Relay server image (minimal)
FROM scratch AS relay
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/relay /relay
EXPOSE 9000
ENTRYPOINT ["/relay"]
CMD ["-port", "9000"]

# Client image
FROM alpine:3.19 AS client
RUN apk --no-cache add ca-certificates
COPY --from=builder /bin/filedrop /usr/local/bin/filedrop
COPY --from=builder /bin/filedrop-tui /usr/local/bin/filedrop-tui
ENTRYPOINT ["filedrop"]
