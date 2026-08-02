# Architecture

## Overview

go-user-tele-downloader is a Telegram user client that monitors channels and automatically downloads files using the MTProto protocol. Unlike the bot-based version, this uses a user account to access channels and download files up to 4GB (with Telegram Premium).

## Architecture Layers

```
┌─────────────────────────────────────────┐
│         cmd/client (Entry Point)        │
└────────────────┬────────────────────────┘
                 │
        ┌────────▼────────┐
        │  internal/client│  ← MTProto client, authentication,
        │                 │    channel monitoring, file downloads
        └────────┬────────┘
                 │
        ┌────────▼────────┐
        │ internal/domain │  ← Business logic, download service
        └────────┬────────┘
                 │
   ┌─────────────┼─────────────┐
   │             │             │
┌──▼──┐    ┌────▼────┐   ┌───▼───┐
│model│    │repository│   │config │
└─────┘    └─────────┘   └───────┘
```

## Components

### 1. MTProto Client (`internal/client`)

Handles the MTProto protocol communication:
- **Authentication**: Phone number + verification code + optional 2FA
- **Session Management**: Encrypted session storage for persistent login
- **Channel Monitoring**: Subscribes to channel updates
- **File Downloads**: Downloads files using MTProto's native download API

Key types:
- `TelegramClient`: Main client wrapper
- `Authenticator`: Handles authentication flow
- `ChannelMonitor`: Monitors channels for new messages
- `FileDownloader`: Downloads files from messages

### 2. Domain Layer (`internal/domain`)

Business logic and orchestration:
- `DownloadService`: Manages download lifecycle
- Coordinates between client and repository layers
- Handles download queue and concurrency

### 3. Model Layer (`internal/model`)

Domain types:
- `Download`: Represents a file download
- `User`: Telegram user information
- `Chat`: Telegram chat/channel information

### 4. Repository Layer (`internal/repository`)

Data persistence:
- `DownloadRepository`: Interface for download storage
- `SQLiteDownloadRepository`: SQLite implementation
- Stores download history, status, and metadata

### 5. Configuration (`internal/config`)

Environment-based configuration:
- MTProto credentials (API ID, API Hash, Phone)
- 2FA password (optional)
- Channel IDs to monitor
- Download settings

## Data Flow

### Authentication Flow

```
1. Load session from encrypted file
   ↓
2. Check if session is valid
   ↓ (if invalid)
3. Request verification code
   ↓
4. User enters code (interactive or from env)
   ↓
5. Handle 2FA if required
   ↓
6. Save encrypted session
```

### Download Flow

```
1. Channel monitor receives new message
   ↓
2. Check if message contains file
   ↓
3. Extract file metadata
   ↓
4. Create download record in database
   ↓
5. Download file using MTProto
   ↓
6. Save file to disk
   ↓
7. Update download status
```

## Key Differences from Bot API Version

| Aspect | Bot API Version | MTProto Version |
|--------|----------------|-----------------|
| Protocol | HTTP Bot API | MTProto (binary protocol) |
| Authentication | Bot token | Phone number + code |
| File size limit | 20MB (50MB with local server) | 4GB (Premium) |
| Channel access | Must be added to channel | User account access |
| Session storage | N/A | Encrypted session file |
| API library | github.com/go-telegram/bot | github.com/gotd/td |

## Security Considerations

### Session Encryption
- Session files are encrypted using AES-256-GCM
- Encryption key derived from user-provided password
- Hardware acceleration via AES-NI when available

### Credential Storage
- API credentials stored in `.env` (gitignored)
- Phone number and 2FA password in environment
- Session file encrypted at rest

### Channel Access
- Only monitors channels user is already a member of
- No ability to join channels automatically
- Respects channel privacy settings

## Database Schema

```sql
CREATE TABLE downloads (
    id TEXT PRIMARY KEY,
    chat_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    url TEXT NOT NULL,
    file_path TEXT,
    status TEXT NOT NULL,
    progress REAL DEFAULT 0,
    error TEXT,
    file_size INTEGER,
    created_at DATETIME,
    updated_at DATETIME,
    completed_at DATETIME
);

CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    username TEXT,
    first_name TEXT,
    last_name TEXT,
    created_at DATETIME,
    updated_at DATETIME
);

CREATE TABLE chats (
    id INTEGER PRIMARY KEY,
    title TEXT,
    type TEXT,
    created_at DATETIME,
    updated_at DATETIME
);
```

## Error Handling

- **Authentication errors**: Prompt for re-authentication
- **Network errors**: Retry with exponential backoff
- **Download errors**: Mark as failed, continue monitoring
- **Session errors**: Re-authenticate and resume

## Future Enhancements

1. **Multiple Channel Support**: Monitor multiple channels simultaneously
2. **File Filtering**: Download only specific file types
3. **Progress Notifications**: Send download progress to a notification channel
4. **Resume Downloads**: Resume interrupted downloads
5. **Web Interface**: Dashboard for monitoring downloads
6. **API Server**: REST API for programmatic control
