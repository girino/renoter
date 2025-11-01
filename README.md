# Nostr Renoter

A Nostr relay wrapper system that provides onion-routing encryption for events. The client wraps events using nested encryption (once per Renoter in the path), and Renoter servers decrypt one layer and forward to the next Renoter or final destination.

## Overview

Renoter implements nested onion-routing encryption for Nostr events:

- **Client**: A khatru relay that intercepts events from normal Nostr clients, wraps them with nested encryption, and forwards to Renoter servers
- **Server**: Renoter nodes that decrypt one layer of encryption and forward the inner event to the next Renoter or final destination

## Features

- Nested encryption using NIP-44 (one layer per Renoter in the path)
- Multi-relay support using `nostr.SimplePool` for redundancy and reliability
- Routing via "p" tags for efficient filtering
- Ephemeral event kinds (29000) for non-persistence
- Automatic path validation
- Extensive debug logging with granular control

## Building

```bash
go build -o renoter-client ./cmd/client
go build -o renoter-server ./cmd/server
```

## Quick Start (Testing)

For easier testing, use the `run.sh` script to start 3 Renoter servers and 1 client from a `.env` file:

1. Copy the example environment file:
```bash
cp example.env .env
```

2. Edit `.env` and set your configuration:
```bash
# Required
RENOTER_RELAYS=wss://relay1.com,wss://relay2.com
RENOTER_PATH=npub1...,npub2...,npub3...  # 3 npubs for 3 Renoters
CLIENT_SERVER_RELAYS=wss://relay1.com,wss://relay2.com

# Optional - Private keys for each Renoter (leave empty to auto-generate)
RENOTER_PRIVATE_KEY_1=
RENOTER_PRIVATE_KEY_2=
RENOTER_PRIVATE_KEY_3=
CLIENT_LISTEN=:8080
VERBOSE=
```

3. Run both client and server:
```bash
./run.sh
```

The script will:
- Verify Go code compiles before running
- Use `go run` to always execute the latest code (no need to rebuild)
- Start 3 Renoter servers in the background (each with separate private keys)
- Start the client in the background
- Wait for Ctrl+C and cleanly kill all processes
- Write logs to `server1.log`, `server2.log`, `server3.log`, and `client.log`

## Usage

### Running a Renoter Server

```bash
# Generate a new identity (uses same relay list for listening and forwarding)
renoter-server -private-key="$(openssl rand -hex 32)" \
  -relays="wss://relay1.com,wss://relay2.com,wss://relay3.com"

# Or load existing private key
renoter-server -private-key="your-private-key-hex" \
  -relays="wss://relay1.com,wss://relay2.com"
```

The server uses the same list of relays for both listening and forwarding, managed by `nostr.SimplePool`.

### Running the Client

```bash
renoter-client \
  -listen=":8080" \
  -path="npub1...,npub2...,npub3..." \
  -server-relays="wss://relay1.com,wss://relay2.com"
```

You can specify multiple server relays for redundancy - events will be published to all of them.

The client runs a Nostr relay on the specified address/port. Connect your Nostr client to it, and events will be automatically wrapped and forwarded through the Renoter path to all specified server relays.

### Debug Logging

Enable verbose logging to see detailed information about event processing:

```bash
# Enable all debug logging
VERBOSE=1 ./renoter-client ...

# Enable specific modules only
VERBOSE=client.wrapper,server.handler ./renoter-server ...

# Or use command-line flag
./renoter-client -verbose=true ...
```

## How It Works

1. **Wrapping (Client)**: Events are wrapped in reverse order of the Renoter path
   - Last Renoter's encryption is the innermost
   - First Renoter's encryption is the outermost
   - Each wrapper event uses ephemeral kind 29000

2. **Unwrapping (Server)**: Each Renoter:
   - Receives wrapped event
   - Verifies signature
   - Decrypts content using its private key
   - Publishes inner event (either another wrapper or the final event)

## Project Structure

```
renoter/
├── cmd/
│   ├── client/          # Client CLI (khatru relay)
│   └── server/          # Server CLI
├── pkg/
│   ├── client/          # Client library
│   │   ├── wrapper.go   # Event wrapping logic
│   │   ├── path.go      # Path validation
│   │   └── relay.go     # Khatru integration
│   └── server/          # Server library
│       ├── renoter.go   # Server struct
│       └── handler.go   # Event processing
└── internal/
    └── config/          # Configuration types
```

## Dependencies

- `github.com/nbd-wtf/go-nostr` - Nostr protocol, NIP-44 encryption, NIP-19 encoding
- `github.com/fiatjaf/khatru` - Nostr relay server for client component

## License

MIT

