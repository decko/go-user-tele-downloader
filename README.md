# go-user-tele-downloader

Telegram user client for automatic file downloads from channels using MTProto protocol.

## Features

- **Automatic Channel Monitoring**: Monitors specified channels for new file messages
- **Large File Support**: Downloads files up to 4GB (with Telegram Premium)
- **MTProto Protocol**: Direct user client, no bot API limitations
- **SQLite Persistence**: Tracks download history and status
- **Headless Operation**: Runs as a daemon without user interaction

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
internal/client/     → MTProto client and channel monitoring
internal/domain/     → Business logic services
internal/model/      → Domain types (Download, User, Chat)
internal/repository/ → Database implementations (SQLite)
internal/config/     → Configuration loading
internal/migration/  → Database migrations
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
| File size limit | Unlimited (local server) | 4GB (Premium) |
| Channel monitoring | ❌ No | ✅ Yes |
| Authentication | Bot token | Phone number |
| Setup complexity | Medium | Medium |
| Requires local server | Yes | No |

## License

Apache License 2.0
