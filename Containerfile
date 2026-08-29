# Build stage
FROM quay.io/fedora/fedora-minimal:44 AS builder

WORKDIR /build

# Install build dependencies (Go toolchain + git)
RUN microdnf install -y golang git ca-certificates && \
    microdnf clean all

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /telecli ./cmd/telecli

# Runtime stage
FROM quay.io/fedora/fedora-minimal:44

# Install runtime dependencies
RUN microdnf install -y ca-certificates tzdata && \
    microdnf clean all && \
    useradd -m -u 1000 telecli

# Create directories: /identity holds the session file, /data holds downloads + db
RUN mkdir -p /data/downloads /identity && \
    chown -R telecli:telecli /data /identity

# Copy binary
COPY --from=builder /telecli /usr/local/bin/telecli

USER telecli

# Environment variables (overridable at runtime)
ENV TELEGRAM_API_ID=""
ENV TELEGRAM_API_HASH=""
ENV TELEGRAM_PHONE=""
ENV TELEGRAM_PASSWORD=""
ENV TELEGRAM_VERIFICATION_CODE=""
ENV SESSION_PATH="/identity/session.enc"
ENV DOWNLOAD_DIR="/data/downloads"
ENV DATABASE_PATH="/data/tele-downloader.db"
ENV MONITOR_CHANNELS=""
ENV MAX_CONCURRENT_DOWNLOADS=3
ENV LOG_LEVEL=info

# /identity: session file (mount your session.enc here)
# /data:     downloads + database
VOLUME ["/identity", "/data"]

ENTRYPOINT ["telecli"]
CMD ["start"]
