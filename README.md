# go-user-tele-downloader

Telegram user client for automatic file downloads from channels using MTProto protocol.

## Features

- **Automatic Channel Monitoring**: Monitors specified channels for new file messages
- **Large File Support**: Downloads files up to 4GB (with Telegram Premium)
- **MTProto Protocol**: Direct user client, no bot API limitations
- **SQLite Persistence**: Tracks download history and status
- **Headless Operation**: Runs as a daemon without user interaction
- **Progress Tracking**: Real-time progress updates with speed calculation
- **Resume Support**: Automatically resumes interrupted downloads
- **Parallel Downloads**: Up to 4x faster with multi-threaded downloads
- **File Verification**: SHA256 hash verification for integrity
- **Session Encryption**: AES-256-GCM encrypted session storage

## Prerequisites

- Go 1.25+
- Telegram account
- API credentials from https://my.telegram.org
- Telegram Premium (for files >2GB, up to 4GB)

## Quick Start

### 1. Get API Credentials

1. Visit https://my.telegram.org
2. Log in with your phone number
3. Go to "API development tools"
4. Create a new application
5. Note your `api_id` and `api_hash`

### 2. Configure

```bash
cp .env.example .env
```

Edit `.env` and fill in:
- `TELEGRAM_API_ID`: Your API ID
- `TELEGRAM_API_HASH`: Your API hash
- `TELEGRAM_PHONE`: Your phone number (with country code)
- `MONITOR_CHANNELS`: Channel IDs to monitor (comma-separated)

### 3. Build and Run

```bash
# Build
make build

# Run migrations
make migrate

# Start the client
make run
```

On first run, you'll be prompted to enter the verification code sent to your phone.

## Channel IDs

To find a channel ID:
1. Forward a message from the channel to @userinfobot
2. The bot will show the channel ID (negative number)
3. Use this ID in `MONITOR_CHANNELS`

## Architecture

```
cmd/client/          → Entry point
internal/client/     → MTProto client, authentication, channel monitoring, download engine
internal/domain/     → Business logic services
internal/model/      → Domain types (Download, User, Chat)
internal/repository/ → Database implementations (SQLite)
internal/config/     → Configuration loading
internal/migration/  → Database migrations
```

## Configuration

### Required Environment Variables
```bash
TELEGRAM_API_ID=123456
TELEGRAM_API_HASH=your_api_hash
TELEGRAM_PHONE=+1234567890
```

### Optional Environment Variables
```bash
# 2FA password (also used for session encryption)
TELEGRAM_PASSWORD=your_2fa_password

# Session encryption (if no 2FA)
SESSION_ENCRYPTION_PASSWORD=your_password

# Headless authentication
TELEGRAM_VERIFICATION_CODE=12345

# Channels to monitor (comma-separated)
MONITOR_CHANNELS=-1001234567890,-1009876543210

# Download settings
DOWNLOAD_DIR=./downloads
MAX_CONCURRENT_DOWNLOADS=3
SESSION_PATH=./session.enc
```

## Radarr Integration (optional)

When configured, completed downloads are automatically imported into your Radarr library:

1. **Lookup** — the file is identified via Radarr's TMDB-backed lookup
2. **Auto-add** — if the movie isn't in Radarr yet, it's added (uses `RADARR_ROOT_FOLDER` and `RADARR_QUALITY_PROFILE_ID`)
3. **Import** — Radarr renames, moves, and triggers your library scan

**Enable it** by setting these in your `.env`:

```bash
RADARR_URL=http://localhost:7878
RADARR_API_KEY=your_key
RADARR_ROOT_FOLDER=/media/movies
RADARR_QUALITY_PROFILE_ID=1
```

**Strict matching** (recommended): only import when the filename matches a movie exactly — title AND year. Prevents weak matches from polluting your library:

```bash
RADARR_IMPORT_STRICT=true
```

Without strict mode, a filename that partially matches a movie (e.g., a documentary named after a movie) may be auto-added and imported under the wrong title. Files that don't match confidently are **left in `downloads/` untouched** — they're never deleted or mis-imported silently.

**Per-file opt-out:** append `#noimport` to the channel message caption — the file downloads normally but stays in `downloads/` without touching Radarr.

**Manual import** of existing files:

```bash
telecli import /data/downloads/Movie.2026.1080p.mkv
telecli import /data/downloads/Some.Folder
```

## CLI Commands

The `telecli` binary provides several commands for managing the download daemon:

### Start the daemon

```bash
telecli start
```

Starts the Telegram download daemon that monitors configured channels and automatically downloads files.

### Show download status

```bash
telecli status
```

Shows a summary of download status including counts of pending, downloading, completed, and failed downloads.

### List downloads

```bash
telecli list
telecli list --limit 10
telecli list --status downloading
telecli list --status failed
```

Lists downloads with detailed information including ID, filename, status, progress, size, and timestamps.

**Flags:**
- `--limit, -l`: Maximum number of downloads to show (default: 20)
- `--status, -s`: Filter by status (pending, downloading, completed, failed, cancelled)

### Cancel a download

```bash
telecli cancel <download-id>
```

Cancels a pending or downloading download by its ID. The download ID can be found using the `list` command.

### Show configuration

```bash
telecli config
```

Shows the current configuration loaded from environment variables and/or config file. Useful for debugging configuration issues.

## Daemon Mode (systemd)

You can run telecli as a systemd service for automatic startup and management.

### Install the service

```bash
make install-service
```

This will:
1. Build the binary
2. Copy the service file to `/etc/systemd/system/`
3. Enable the service to start on boot

### Manage the service

```bash
# Start the service
sudo systemctl start telecli

# Stop the service
sudo systemctl stop telecli

# Check service status
sudo systemctl status telecli

# View logs
sudo journalctl -u telecli -f

# Restart the service
sudo systemctl restart telecli
```

### Uninstall the service

```bash
make uninstall-service
```

## Development

```bash
# Run tests
make test

# Run linter
make lint

# Development mode
make dev
```

## Comparison with go-tele-downloader

| Feature | go-tele-downloader (Bot API) | go-user-tele-downloader (MTProto) |
|---------|------------------------------|-----------------------------------|
| Protocol | Bot API | MTProto (User API) |
| File size limit | 20MB (local server) | 4GB (Premium) |
| Channel monitoring | ❌ No | ✅ Yes |
| Authentication | Bot token | Phone number |
| Setup complexity | Medium (container) | Medium |
| Requires local server | Yes | No |
| Resume support | No | Yes |
| Parallel downloads | No | Yes (4 threads) |
| Progress tracking | Basic | Real-time |

## Implementation Phases

- ✅ **Phase 1**: Research & Foundation - Evaluated gotd/td library, created PoC
- ✅ **Phase 2**: Authentication System - Interactive/headless auth, session encryption
- ✅ **Phase 3**: Channel Monitoring - Update subscription, message filtering
- ✅ **Phase 4**: File Download Engine - Progress tracking, resume, parallel downloads
- ⏳ **Phase 5**: CLI & Daemon Mode - Command-line interface, systemd integration (planned)

See [docs/implementation-summary.md](docs/implementation-summary.md) for detailed implementation notes.

## Performance

| Metric | Value |
|--------|-------|
| Max File Size | 4GB (with Premium) |
| Download Speed | Up to 4x faster with parallel downloads |
| Concurrent Downloads | Configurable (default: 3) |
| Resume Support | Yes, automatic |
| Progress Updates | Every 1 second (configurable) |

## Security

1. **Session Encryption**: AES-256-GCM with PBKDF2 key derivation
2. **Hardware Acceleration**: AES-NI automatically used when available
3. **File Verification**: SHA256 hash verification
4. **Secure Storage**: Session file with 0600 permissions
5. **Channel Access**: Only monitors channels user is member of

## Troubleshooting

### Authentication Failed
- Verify API credentials from https://my.telegram.org
- Check phone number format (with country code)
- Ensure 2FA password is correct if enabled

### Session Expired
- Delete session file: `rm session.enc`
- Re-authenticate with `make run`

### Download Failed
- Check channel access (must be member)
- Verify file size (max 4GB with Premium)
- Check network connectivity
- Review logs for specific error

## License

Apache License 2.0
