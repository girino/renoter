# Nostr Renoter

**Version: 1.0.0-alpha**

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
- Proof-of-work (PoW) for spam protection: All 29000 wrapper events require PoW with difficulty 16 (~65K attempts on average)
- Replay attack protection with bounded in-memory cache (max 5K entries)
- Configurable cache cutoff duration (default: 2 hours)
- Extensive debug logging with granular control
- Docker support for production deployment

## Building from Source

```bash
# Build client
go build -o renoter-client ./cmd/client

# Build server
go build -o renoter-server ./cmd/server
```

## Docker Deployment

### Prerequisites

- Docker and Docker Compose installed
- Environment variables configured (see below)

### Server Deployment

1. Copy the server environment file:
```bash
cp example.env.server .env
```

2. Edit `.env` and configure:
   - `RENOTER_PRIVATE_KEY`: Your server's private key in hex format (leave empty to auto-generate)
   - `RENOTER_RELAYS`: Comma-separated relay URLs (e.g., `wss://relay1.com,wss://relay2.com`)
   - `VERBOSE`: Optional debug logging (set to `true`, `all`, or module-specific like `server.handler,server.cache`)

3. Build and run:
```bash
docker-compose -f docker-compose.server.yml up -d
```

4. View logs:
```bash
docker-compose -f docker-compose.server.yml logs -f
```

5. Stop the server:
```bash
docker-compose -f docker-compose.server.yml down
```

### Client Deployment

1. Copy the client environment file:
```bash
cp example.env.client .env
```

2. Edit `.env` and configure:
   - `RENOTER_PATH`: Comma-separated npubs of Renoter servers in the path (e.g., `npub1...,npub2...,npub3...`)
   - `CLIENT_SERVER_RELAYS`: Comma-separated relay URLs where wrapped events will be sent
   - `CLIENT_LISTEN`: Listen address (default: `:8080`)
   - `CLIENT_PORT`: Docker port mapping (default: `8080`)
   - `VERBOSE`: Optional debug logging (set to `true`, `all`, or module-specific like `client.wrapper,client.relay`)

3. Build and run:
```bash
docker-compose -f docker-compose.client.yml up -d
```

4. View logs:
```bash
docker-compose -f docker-compose.client.yml logs -f
```

5. Stop the client:
```bash
docker-compose -f docker-compose.client.yml down
```

### Getting Server Public Keys

After starting a server, you can find its public key (npub) in the logs:
```bash
docker-compose -f docker-compose.server.yml logs | grep "Created Renoter instance"
```

Or run the server once with auto-generation to see the generated keys:
```bash
docker-compose -f docker-compose.server.yml run --rm renoter-server
```

## Local Testing (Development)

For easier local testing and development, use the `run.sh` script to start 3 Renoter servers and 1 client:

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

3. Run both client and servers:
```bash
./run.sh
```

The script will:
- Verify Go code compiles before running
- Clean previous builds and build fresh executables
- Start 3 Renoter servers in the background (each with separate private keys)
- Start the client in the background
- Write logs to `server1.log`, `server2.log`, `server3.log`, and `client.log`
- Wait for Ctrl+C and cleanly kill all processes

4. View logs:
```bash
tail -f server1.log
tail -f client.log
```

## Manual Usage

### Running a Renoter Server

```bash
# Generate a new identity
renoter-server -relays="wss://relay1.com,wss://relay2.com"

# Or use existing private key
renoter-server \
  -private-key="your-private-key-hex" \
  -relays="wss://relay1.com,wss://relay2.com"
```

**Server Flags:**
- `-relays`: Comma-separated relay URLs (required)
- `-private-key`: Private key in hex format (optional, auto-generates if not provided)
- `-verbose`: Verbose logging level (optional)

The server uses the same list of relays for both listening and forwarding, managed by `nostr.SimplePool`.

### Running the Client

```bash
renoter-client \
  -listen=":8080" \
  -path="npub1...,npub2...,npub3..." \
  -server-relays="wss://relay1.com,wss://relay2.com"
```

**Client Flags:**
- `-listen`: Listen address for the khatru relay (default: `:8080`)
- `-path`: Comma-separated npubs of Renoter servers in the path (required)
- `-server-relays`: Comma-separated relay URLs where wrapped events will be sent (required)
- `-verbose`: Verbose logging level (optional)

You can specify multiple server relays for redundancy - events will be published to all of them.

The client runs a Nostr relay on the specified address/port. Connect your Nostr client to it, and events will be automatically wrapped and forwarded through the Renoter path to all specified server relays.

### Debug Logging

Enable verbose logging to see detailed information about event processing:

**Environment Variable:**
```bash
# Enable all debug logging
export VERBOSE=true

# Enable specific modules only
export VERBOSE=client.wrapper,server.handler,server.cache
```

**Command-Line Flag:**
```bash
./renoter-client -verbose=true ...
./renoter-server -verbose=server.handler ...
```

**Available Logging Modules:**
- `client.wrapper`: Event wrapping logic
- `client.relay`: Khatru relay integration
- `client.path`: Path validation
- `server.handler`: Event handling and decryption
- `server.renoter`: Renoter server core logic
- `server.cache`: Replay cache operations

## How It Works

### Event Wrapping (Client)

1. Normal Nostr client publishes event to khatru relay (Renoter client)
2. Client intercepts the event via `RejectEvent` hook
3. Client creates nested wrapper events in **reverse order** of the Renoter path:
   - Last Renoter's encryption is the innermost
   - First Renoter's encryption is the outermost
   - Each wrapper event uses ephemeral kind 29000
   - Each wrapper includes a "p" tag with the destination Renoter's pubkey for routing
   - Each 29000 wrapper event is mined with proof-of-work (difficulty 16) before signing
4. Client pads the outermost 29000 event to a standardized size (32KB) and wraps it in a 29001 container
5. Client publishes the final wrapped event (29001) to all specified server relays

### Event Unwrapping (Server)

1. Renoter server subscribes to wrapper events (kind 29001) with its pubkey in "p" tag
2. Receives wrapped event (29001) and verifies signature
3. Decrypts the 29001 event to get the inner 29000 event
4. Validates proof-of-work for the 29000 event (checks committed difficulty >= 16)
5. Checks replay attack protection (rejects if already seen)
6. Decrypts the 29000 event content using its private key (NIP-44)
7. Deserializes inner event (either another 29000 wrapper or the final event)
8. If inner event is another 29000, validates its PoW and re-wraps it for the next Renoter
9. Publishes inner event to all configured relays

### Replay Attack Protection

The server maintains an in-memory cache of processed event IDs:
- Maximum 5K entries (configurable)
- Events older than 2 hours are automatically cleaned up (configurable)
- Uses binary search for efficient cleanup
- Events with `CreatedAt` more than 1 hour in the past are rejected
- Cache pruning removes 25% of oldest entries when limit is reached

## Project Structure

```
renoter/
├── cmd/
│   ├── client/          # Client CLI tool (khatru relay)
│   │   └── main.go
│   └── server/          # Server CLI tool
│       └── main.go
├── pkg/
│   ├── client/          # Client library
│   │   ├── wrapper.go   # Event wrapping logic
│   │   ├── path.go      # Path validation
│   │   └── relay.go     # Khatru integration
│   └── server/          # Server library
│       ├── renoter.go   # Renoter server logic
│       ├── handler.go   # Event handling and decryption
│       └── cache.go     # Replay attack protection cache
├── internal/
│   └── config/          # Configuration types
│       └── config.go
├── Dockerfile.client     # Docker build for client
├── Dockerfile.server     # Docker build for server
├── docker-compose.client.yml  # Docker compose for client
├── docker-compose.server.yml  # Docker compose for server
├── example.env           # Example config for local testing
├── example.env.server    # Example config for server deployment
├── example.env.client    # Example config for client deployment
├── run.sh                # Local testing script
└── README.md
```

## Configuration

### Environment Variables

**Server:**
- `RENOTER_PRIVATE_KEY`: Private key in hex format (optional, auto-generates if empty)
- `RENOTER_RELAYS`: Comma-separated relay URLs
- `VERBOSE`: Debug logging level

**Client:**
- `RENOTER_PATH`: Comma-separated npubs of Renoter servers
- `CLIENT_SERVER_RELAYS`: Comma-separated relay URLs for publishing wrapped events
- `CLIENT_LISTEN`: Listen address (default: `:8080`)
- `CLIENT_PORT`: Docker port mapping (default: `8080`)
- `VERBOSE`: Debug logging level

### Key Generation

If you don't provide a private key, the server will generate one automatically. To get the corresponding public key (npub), check the server logs:

```bash
# Server logs will show:
# "Created Renoter instance, pubkey: <pubkey> (first 16 chars), X relays"
```

You can also use Go's standard library or Nostr tools to derive the npub from a private key.

## Security Considerations

- **Proof-of-Work**: All 29000 wrapper events require PoW (difficulty 16) to prevent spam attacks
- **Replay Protection**: Events are cached and rejected if processed twice (within the cache window)
- **Age Validation**: Events older than 1 hour are automatically rejected
- **Ephemeral Events**: Wrapper events use kind 29000/29001 and are marked as non-persistent
- **Standardized Sizes**: Messages are padded to fixed sizes (32KB) to prevent metadata leakage
- **Private Keys**: Never commit private keys to version control. Use environment variables or secure key management.
- **Network**: Ensure secure connections (WSS) to relays

## Troubleshooting

### Client fails to start
- Verify `RENOTER_PATH` contains valid npubs (comma-separated, no spaces)
- Check that `CLIENT_SERVER_RELAYS` are accessible
- Ensure relays are online and reachable

### Server not receiving events
- Verify server's public key is in the client's `RENOTER_PATH`
- Check that relay URLs in `RENOTER_RELAYS` are correct
- Ensure the server is subscribed to wrapper events (kind 29000)
- Check server logs for subscription confirmation

### Events not being forwarded
- Verify signature validation passes (check logs)
- Check replay protection isn't rejecting valid events
- Ensure inner events are being published to relays
- Verify relay connections are active

### Debug Logging
Enable verbose logging to troubleshoot:
```bash
VERBOSE=all ./renoter-server ...
VERBOSE=client.wrapper,server.handler ./renoter-client ...
```

## License

[Add your license here]

## Contributing

[Add contribution guidelines here]
