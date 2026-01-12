.PHONY: build build-client build-relay build-tui run-relay run-tui docker-build docker-up docker-down clean

# Build all
build: build-client build-relay build-tui

# Build client binary
build-client:
	go build -o bin/filedrop ./cmd/client

# Build relay binary
build-relay:
	go build -o bin/relay ./cmd/relay

# Build TUI binary
build-tui:
	go build -o bin/filedrop-tui ./cmd/tui

# Run TUI
run-tui:
	go run ./cmd/tui

# Run relay locally
run-relay:
	go run ./cmd/relay -port 9000

# Run relay with ngrok (public access)
run-relay-public:
	./scripts/run-ngrok-relay.sh

# Docker commands
docker-build:
	docker-compose build

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f relay

# Cross-compile for different OS
build-all:
	@mkdir -p bin
	# Linux
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/filedrop-linux-amd64 ./cmd/client
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/filedrop-linux-arm64 ./cmd/client
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/filedrop-tui-linux-amd64 ./cmd/tui
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/filedrop-tui-linux-arm64 ./cmd/tui
	# macOS
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/filedrop-darwin-amd64 ./cmd/client
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/filedrop-darwin-arm64 ./cmd/client
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/filedrop-tui-darwin-amd64 ./cmd/tui
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/filedrop-tui-darwin-arm64 ./cmd/tui
	# Windows
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/filedrop-windows-amd64.exe ./cmd/client
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/filedrop-tui-windows-amd64.exe ./cmd/tui
	@echo "✅ Built for all platforms"

# Clean build artifacts
clean:
	rm -rf bin/

# Install client to system
install: build-client
	sudo cp bin/filedrop /usr/local/bin/

# Test transfer locally
test-local:
	@echo "Starting relay..."
	@go run ./cmd/relay -port 9000 &
	@sleep 1
	@echo "Test with: ./bin/filedrop send <file>"
