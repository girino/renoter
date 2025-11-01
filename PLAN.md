# Nostr Renoter Implementation Plan

## Overview
Implement a complete Nostr Renoter system with both client (relay that wraps events) and server (relaying) components. The client uses khatru relay to receive events from normal Nostr clients and wraps them in Renoter format. The system uses nested onion-routing encryption where events are encrypted multiple times (once for each Renoter in the path) using NIP-44 encryption from go-nostr library. No routing tags are needed - each Renoter decrypts one layer to reveal the next encrypted layer or the final event.

## Project Structure
```
renoter/
├── go.mod
├── README.md
├── cmd/
│   ├── client/          # Client CLI tool (khatru relay)
│   │   └── main.go
│   └── server/          # Renoter server
│       └── main.go
├── pkg/
│   ├── client/          # Client library
│   │   ├── wrapper.go   # Event wrapping logic
│   │   ├── path.go      # Path selection
│   │   └── relay.go     # Khatru integration
│   └── server/          # Server library
│       ├── renoter.go   # Renoter server logic
│       └── handler.go   # Event handling
└── internal/
    └── config/          # Configuration types
        └── config.go
```

## Implementation Todos

1. **Project Setup**: Initialize Go module, create directory structure, add dependencies (go-nostr, khatru), and add README
2. **Core Data Structures and Utilities**: Define wrapper event format convention, use go-nostr library directly for events and encryption
3. **Client Library**: Implement wrapper.go (nested event wrapping), path.go (path validation), and relay.go (khatru integration)
4. **Server Library**: Implement renoter.go (server struct and ProcessEvent) and handler.go (decrypt, deserialize, publish)
5. **Client CLI**: Build command-line tool using khatru relay that intercepts events, wraps them, and forwards to Renoter server
6. **Server CLI**: Build command-line tool that generates/loads Renoter identity, listens for wrapped events, and processes/forwards them
7. **Integration Points**: Verify and document usage of go-nostr library functions (relay connections, encryption, signing, NIP-19 encoding/decoding) and khatru integration

## Implementation Steps

### 1. Project Setup
- Initialize Go module (`go mod init renoter`)
- Add dependencies:
  - `github.com/nbd-wtf/go-nostr` for Nostr protocol support (includes NIP-44, NIP-19, etc.)
  - `github.com/fiatjaf/khatru` for Nostr relay server (client component)
- Create directory structure
- Add basic README with usage instructions

### 2. Core Data Structures and Utilities
- Use `github.com/nbd-wtf/go-nostr` library directly for:
  - `nostr.Event` structure (standard Nostr event)
  - `nostr.GeneratePrivateKey()` for random key generation
  - `nostr.GetPublicKey()` for public key derivation
  - `nip44.Encrypt()` and `nip44.Decrypt()` for NIP-44 encryption
  - `nip19.EncodePublicKey()` and `nip19.Decode()` for npub encoding/decoding
- Define wrapper event format convention:
  - No routing tags needed - nested encryption handles routing automatically
  - Encrypted content (NIP-44 format) in event content field
  - Use ephemeral event kind (20000-29999 range, e.g., 29000) for wrapper events to ensure non-persistence
  - Events should be marked as non-persistent to prevent relays from storing them

### 3. Client Library (`pkg/client/`)
- `wrapper.go`:
  - `WrapEvent()`: Main function that takes original `nostr.Event` and Renoter path (npubs)
  - Create wrapper events for each Renoter in reverse order (last Renoter first), building nested structure
  - Example for 3 Renoters R1, R2, R3:
    - Step 1: Create wrapper event for R3:
      - Serialize original event to JSON
      - Encrypt for R3: `ciphertext_R3 = nip44.Encrypt(npub_R3, original_event_json)`
      - Create `nostr.Event` wrapper_R3 with `ciphertext_R3` as content
      - Generate random key `sk3`, sign wrapper_R3 with `sk3`
    - Step 2: Create wrapper event for R2:
      - Serialize wrapper_R3 to JSON
      - Encrypt for R2: `ciphertext_R2 = nip44.Encrypt(npub_R2, wrapper_R3_json)`
      - Create `nostr.Event` wrapper_R2 with `ciphertext_R2` as content
      - Generate random key `sk2`, sign wrapper_R2 with `sk2`
    - Step 3: Create wrapper event for R1 (final):
      - Serialize wrapper_R2 to JSON
      - Encrypt for R1: `ciphertext_R1 = nip44.Encrypt(npub_R1, wrapper_R2_json)`
      - Create `nostr.Event` wrapper_R1 with `ciphertext_R1` as content
      - Generate random key `sk1`, sign wrapper_R1 with `sk1`
  - Return final wrapper event (wrapper_R1) to be published
  - Set event kind to ephemeral (e.g., 29000) for non-persistence on all wrapper events
- `path.go`:
  - Path validation (ensure valid npubs using `nip19.Decode()`)
  - Path selection helpers
- `relay.go`:
  - Integration with khatru relay
  - Event handler that intercepts incoming events from normal Nostr clients
  - Automatically wraps events using `WrapEvent()` and forwards to Renoter server

### 4. Server Library (`pkg/server/`)
- `renoter.go`:
  - Renoter server struct with:
    - Private key (this Renoter's identity)
    - Public key (derived from private key)
    - Event store (for replay detection - basic in-memory map of event IDs)
    - Relay connection for listening and forwarding
  - `ProcessEvent()`: Main processing function
- `handler.go`:
  - Receive wrapped `nostr.Event`
  - Verify signature using `ev.CheckSignature()`
  - Decrypt content using `nip44.Decrypt()` with Renoter's private key
  - Deserialize decrypted content to get inner `nostr.Event`
  - Publish inner event to Nostr relay (this will either be another wrapper event for the next Renoter, or the final original event)

### 5. Client CLI (`cmd/client/main.go`)
- Command-line tool that runs a Nostr relay using khatru:
  - Accepts events from any normal Nostr client
  - Intercepts/wraps received events in Renoter format using nested encryption
  - Forwards wrapped events to Renoter server
- Flags for:
  - Listen address/port (for relay HTTP/WebSocket)
  - Renoter path (comma-separated npubs)
  - Renoter server relay URL (where to send wrapped events)
  - Config file support

### 6. Server CLI (`cmd/server/main.go`)
- Command-line tool to:
  - Generate/load Renoter identity (private key)
  - Listen for wrapped events (via Nostr relay subscription)
  - Process and forward events
  - Publish final events to Nostr network
- Flags for:
  - Private key (or generate new)
  - Relay URL to listen on
  - Relay URL to forward to
  - Config file support

### 7. Integration Points
- Use `github.com/nbd-wtf/go-nostr` library for:
  - Relay connections (`nostr.RelayConnect()`)
  - Event publishing (`relay.Publish()`)
  - Event subscription/filtering (`relay.Subscribe()`)
  - Event signing (`ev.Sign(sk)`)
  - Signature verification (`ev.CheckSignature()`)
  - NIP-44 encryption (`nip44.Encrypt()` / `nip44.Decrypt()`)
  - Key generation (`nostr.GeneratePrivateKey()`, `nostr.GetPublicKey()`)
  - NIP-19 encoding/decoding (`nip19.EncodePublicKey()`, `nip19.Decode()`) for npub handling
- Use `github.com/fiatjaf/khatru` for client relay functionality
- Use ephemeral event kind (e.g., 29000) for wrapper events to ensure non-persistence

## Technical Details

### Event Wrapping Format
- No routing tags needed - nested encryption handles routing automatically
- Content field contains nested encrypted data (NIP-44 format)
- Events are serialized to JSON before encryption for nested wrapping
- Use ephemeral event kinds (20000-29999) for non-persistence

### Encryption Flow
1. Normal Nostr client publishes event to khatru relay (client)
2. Client intercepts original event
3. Client creates wrapper event for R3 (last Renoter):
   - Serialize original event to JSON
   - Encrypt for R3: `ciphertext_R3 = nip44.Encrypt(npub_R3, original_event_json)`
   - Create wrapper_R3 event with `ciphertext_R3` as content
   - Generate random key sk3, sign wrapper_R3 with sk3
4. Client creates wrapper event for R2:
   - Serialize wrapper_R3 event to JSON
   - Encrypt for R2: `ciphertext_R2 = nip44.Encrypt(npub_R2, wrapper_R3_json)`
   - Create wrapper_R2 event with `ciphertext_R2` as content
   - Generate random key sk2, sign wrapper_R2 with sk2
5. Client creates wrapper event for R1 (first Renoter, final wrapper):
   - Serialize wrapper_R2 event to JSON
   - Encrypt for R1: `ciphertext_R1 = nip44.Encrypt(npub_R1, wrapper_R2_json)`
   - Create wrapper_R1 event with `ciphertext_R1` as content
   - Generate random key sk1, sign wrapper_R1 with sk1
6. Client publishes final wrapper event (wrapper_R1) to Renoter server

### Decryption Flow (Renoter)
1. Receives wrapped event, verifies signature using `ev.CheckSignature()`
2. Decrypts content using `nip44.Decrypt()` with own private key
3. Deserializes decrypted content to get inner `nostr.Event`
4. Publishes inner event to Nostr relay (same action whether it's another wrapper event or the final original event)

## Testing Considerations
- Unit tests for nested encryption/decryption using library functions
- Integration tests with mock relays
- End-to-end test with multiple Renoter instances
- Test khatru relay integration
- Verify non-persistence of wrapper events

## Future Enhancements (Not in Initial Implementation)
- Cashu token integration
- Expiration tags (NIP-40)
- Replay attack protection (persistent storage)
- Padding mechanism
- NIP-70 protected events
- Random delays and dummy packets

