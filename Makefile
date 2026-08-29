.PHONY: build run dev test lint clean migrate install-service

BINARY=telecli
GOFLAGS=-trimpath
LDFLAGS=-s -w

# Build the binary
build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/telecli

# Run the binary
run:
	./bin/$(BINARY) start

# Run in development mode (with hot reload)
dev:
	go run ./cmd/telecli start

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
	go run ./cmd/telecli migrate

# Install systemd service
install-service: build
	sudo cp systemd/telecli.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable telecli
	@echo "Service installed. Start with: sudo systemctl start telecli"
	@echo "Check status with: sudo systemctl status telecli"
	@echo "View logs with: sudo journalctl -u telecli -f"

# Uninstall systemd service
uninstall-service:
	sudo systemctl stop telecli || true
	sudo systemctl disable telecli || true
	sudo rm -f /etc/systemd/system/telecli.service
	sudo systemctl daemon-reload
	@echo "Service uninstalled"
