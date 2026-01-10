FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build relay server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /relay ./cmd/relay

# Build client
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /filedrop ./cmd/client

# Relay server image
FROM alpine:latest AS relay
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /relay /usr/local/bin/relay
EXPOSE 9000
CMD ["relay", "-port", "9000"]

# Client image
FROM alpine:latest AS client
RUN apk --no-cache add ca-certificates
COPY --from=builder /filedrop /usr/local/bin/filedrop
ENTRYPOINT ["filedrop"]
