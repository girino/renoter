# Nostr Renoter

A Nostr relay wrapper system that provides onion-routing encryption for events. The client wraps events using nested encryption (once per Renoter in the path), and Renoter servers decrypt one layer and forward to the next Renoter or final destination.

## Overview

Renoter implements nested onion-routing encryption for Nostr events:

- **Client**: A khatru relay that intercepts events from normal Nostr clients, wraps them with nested encryption, and forwards to Renoter servers
- **Server**: Renoter nodes that decrypt one layer of encryption and forward the inner event to the next Renoter or final destination

## Features

- Nested encryption using NIP-44 (one layer per Renoter in the path)
- No routing tags needed - encryption handles routing automatically
- Ephemeral event kinds (29000) for non-persistence
- Automatic path validation

## Building

```bash
go build -o renoter-client ./cmd/client
go build -o renoter-server ./cmd/server
```

## Usage

### Running a Renoter Server

```bash
# Generate a new identity
renoter-server -private-key="$(openssl rand -hex 32)" \
  -listen-relay="wss://your-relay.com" \
  -forward-relay="wss://next-relay.com"

# Or load existing private key
renoter-server -private-key="your-private-key-hex" \
  -listen-relay="wss://your-relay.com" \
  -forward-relay="wss://next-relay.com"
```

### Running the Client

```bash
renoter-client \
  -listen=":8080" \
  -path="npub1...,npub2...,npub3..." \
  -server-relay="wss://first-renoter-relay.com"
```

The client runs a Nostr relay on the specified address/port. Connect your Nostr client to it, and events will be automatically wrapped and forwarded through the Renoter path.

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

