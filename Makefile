.PHONY: build run dev test lint clean migrate

BINARY=tele-user-downloader
GOFLAGS=-trimpath
LDFLAGS=-s -w

# Build the binary
build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/client

# Run the binary
run:
	./bin/$(BINARY)

# Run in development mode (with hot reload)
dev:
	go run ./cmd/client

# Run tests
test:
	go test -race -cover ./...

# Run linter
lint:
	golangci-lint run

# Clean build artifacts
clean:
	rm -rf bin/

# Run database migrations
migrate:
	go run ./cmd/client migrate
