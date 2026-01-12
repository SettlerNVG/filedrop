# FileDrop

[![CI](https://github.com/SettlerNVG/filedrop/workflows/CI/badge.svg)](https://github.com/SettlerNVG/filedrop/actions)
[![codecov](https://codecov.io/gh/SettlerNVG/filedrop/branch/main/graph/badge.svg)](https://codecov.io/gh/SettlerNVG/filedrop)
[![Go Report Card](https://goreportcard.com/badge/github.com/SettlerNVG/filedrop)](https://goreportcard.com/report/github.com/SettlerNVG/filedrop)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

P2P file sharing via relay server. Works across any OS (Linux, macOS, Windows).

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/SettlerNVG/filedrop/main/install.sh | bash
```

Or download binaries from [Releases](https://github.com/SettlerNVG/filedrop/releases).

## Features

- ✅ Transfer files of any size (streaming, low memory usage)
- ✅ Transfer folders with preserved structure
- ✅ AES-256-GCM encryption (end-to-end)
- ✅ Gzip compression
- ✅ Resume interrupted transfers
- ✅ Progress bar with speed
- ✅ TUI interface — see online users and transfer files
- ✅ Auto-connect to public relay

## Quick Start

Client automatically connects to the public relay server (address updates dynamically).

### TUI Mode (recommended)

```bash
filedrop-tui
```

In TUI you can:
- See all online users
- Select a user and send files
- Accept incoming transfers

### CLI Mode

```bash
# Send a file
filedrop send myfile.zip
# You'll get a code: A1B2C3

# Receive a file
filedrop receive A1B2C3
```

## How It Works

```
┌──────────────┐                ┌──────────────┐                ┌──────────────┐
│    Sender    │ ──── TCP ────► │    Relay     │ ◄──── TCP ──── │   Receiver   │
│              │   encrypted    │   (ngrok)    │   encrypted    │              │
└──────────────┘                └──────────────┘                └──────────────┘
```

1. Owner runs relay with public access via ngrok
2. Relay address is automatically published to this repository
3. Clients fetch the current address on startup and connect

## Using Custom Relay

```bash
# Via flag
filedrop -relay myserver:9000 send file.zip
filedrop-tui -relay myserver:9000

# Via environment variable
export FILEDROP_RELAY=myserver:9000

# Via config file
echo "myserver:9000" > ~/.filedrop/relay
```

## Running Your Own Relay

### Local (for testing)

```bash
make run-relay
```

### Public (via ngrok)

Requires ngrok with a linked card for TCP tunnels.

```bash
# Install
brew install ngrok
ngrok config add-authtoken <your-token>

# Run
make run-relay-public
```

The script will automatically:
- Start the relay server
- Create ngrok tunnel
- Publish address to repository
- Clients will auto-connect

## Flags

| Flag | Description |
|------|-------------|
| `-relay` | Relay server address |
| `-output` | Output directory |
| `-password` | Encryption password |
| `-compress` | Enable compression |

## TUI Controls

| Key | Action |
|-----|--------|
| `1` / `u` | List online users |
| `2` / `p` | Pending transfers |
| `r` | Refresh |
| `Enter` | Select |
| `Esc` | Back |
| `q` | Quit |

## License

Apache 2.0
