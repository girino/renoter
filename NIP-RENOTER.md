# NIP-XX: Renoter (Onion-Routing for Nostr Events)

`draft` `optional` `author:girino` `author:github:girino`

## Abstract

This NIP defines a protocol for onion-routing encrypted events through Nostr relays using nested encryption. The protocol enables clients to route events through a chain of Renoter nodes, providing metadata privacy and anonymization through layered encryption. Each Renoter decrypts one layer and forwards the inner event to the next node or final destination.

## Motivation

Nostr's open relay model exposes metadata such as sender identities, recipient information, and event relationships to relay operators. Renoter addresses this by:

1. **Metadata Privacy**: Relay operators cannot determine the original sender or final destination of events
2. **Anonymous Routing**: Events are routed through intermediate nodes, breaking direct sender-receiver links
3. **End-to-End Encryption**: Original events are encrypted before entering the Renoter network
4. **Standardized Protocol**: Provides a standard way to implement onion-routing for Nostr events

## Specification

### Event Kinds

Two ephemeral event kinds are used (ephemeral events in range 20000-29999 are non-persistent per [NIP-16](https://github.com/nostr-protocol/nips/blob/master/16.md)):

- **`29000`**: Wrapper event kind for routing layer events. Contains encrypted inner events and requires proof-of-work.
- **`29001`**: Standardized wrapper event kind for outer container. Contains a padded 29000 event encrypted to exactly 32KB to prevent metadata leakage.

### Wrapper Event Structure (Kind 29000)

A wrapper event has the following structure:

```json
{
  "id": "<event-id>",
  "pubkey": "<random-generated-pubkey>",
  "created_at": <unix-timestamp>,
  "kind": 29000,
  "tags": [
    ["p", "<destination-renoter-pubkey-hex>"],
    ["nonce", "<nonce>", "<committed-difficulty>"]
  ],
  "content": "<nip44-encrypted-json-string>",
  "sig": "<signature>"
}
```

**Fields:**
- `pubkey`: Randomly generated public key for this wrapper layer (not the sender's key)
- `content`: NIP-44 encrypted JSON string containing either:
  - Another 29000 wrapper event (serialized to JSON), or
  - The final destination event (serialized to JSON)
- `tags["p"]`: Contains the destination Renoter's public key (hex-encoded) for routing
- `tags["nonce"]`: Proof-of-work nonce tag per [NIP-13](https://github.com/nostr-protocol/nips/blob/master/13.md). Required format: `["nonce", "<nonce>", "<committed-difficulty>"]`. The committed difficulty must be >= 16.

### Standardized Wrapper Event Structure (Kind 29001)

A standardized wrapper event contains an encrypted 29000 event padded to exactly 32KB:

```json
{
  "id": "<event-id>",
  "pubkey": "<sender-pubkey>",
  "created_at": <unix-timestamp>,
  "kind": 29001,
  "tags": [
    ["p", "<first-renoter-pubkey-hex>"]
  ],
  "content": "<nip44-encrypted-29000-event-padded-to-32kb>",
  "sig": "<signature>"
}
```

**Fields:**
- `content`: NIP-44 encrypted JSON string containing a 29000 event that has been padded to exactly 32KB (32768 bytes) before encryption
- `tags["p"]`: Contains the first Renoter's public key (hex-encoded) for initial routing

### Event Wrapping Process

The client wraps events in **reverse order** of the Renoter path:

1. Start with the original event
2. For each Renoter in the path (starting from the last):
   a. Serialize the current event (original or previous wrapper) to JSON
   b. Generate a random private key `sk` for this wrapper layer
   c. Generate conversation key: `conversationKey = NIP44.GenerateConversationKey(renoterPubkey, sk)`
   d. Encrypt the JSON: `ciphertext = NIP44.Encrypt(eventJSON, conversationKey)`
   e. Create wrapper event (kind 29000) with:
      - `content`: ciphertext
      - `tags["p"]`: destination Renoter's pubkey (hex)
      - `pubkey`: derived from `sk`
   f. Mine proof-of-work: Add `tags["nonce"]` with difficulty >= 16 using [NIP-13](https://github.com/nostr-protocol/nips/blob/master/13.md)
   g. Compute ID and sign with `sk`
   h. Set current event = wrapper event
3. After creating the outermost 29000 wrapper:
   a. Check if the serialized size exceeds 32KB (32768 bytes). If so, reject with error.
   b. Pad the event to exactly 32KB using padding tags
   c. Serialize the padded 29000 event to JSON
   d. Encrypt it with the first Renoter's pubkey to create a 29001 event
   e. Publish the 29001 event to relays

**Padding Format:**
Padding is added via tags: `["padding", "<random-hex-string>"]`. The padding string length is calculated to make the total serialized event size exactly 32KB. Padding tags are removed after decryption.

### Event Unwrapping Process (Renoter Server)

When a Renoter receives a 29001 event:

1. Verify signature using `event.CheckSignature()`
2. Decrypt the 29001 content using the Renoter's private key (NIP-44)
3. Deserialize the decrypted content to get the inner 29000 event
4. Verify the inner 29000 event:
   - Check `tags["p"]` contains this Renoter's pubkey (if not, silently drop)
   - Validate proof-of-work: `NIP13.CommittedDifficulty(event) >= 16`
   - Check for replay attacks (reject if event ID already seen)
   - Validate event age (reject events that are too old)
5. Decrypt the 29000 content using the Renoter's private key (NIP-44)
6. Remove padding tags from the decrypted event
7. Deserialize to get inner event
8. If inner event is another 29000:
   - Validate its proof-of-work (committed difficulty >= 16)
   - Pad it to 32KB if needed
   - Re-encrypt to create a new 29001 event for the next Renoter
   - Publish to relays
9. If inner event is a final event (any other kind):
   - Publish the final event to relays

### Path Validation

Clients must validate Renoter paths:

1. Each entry must be a valid `npub` (per [NIP-19](https://github.com/nostr-protocol/nips/blob/master/19.md))
2. No duplicate Renoters allowed in the path
3. Path cannot be empty

### Routing Tags

The `["p", "<pubkey-hex>"]` tag is used for routing:
- In 29001 events: Points to the first Renoter in the path
- In 29000 events: Points to the next Renoter in the path (or final destination if last)
- Renoters filter events by subscribing to those with their pubkey in the `p` tag

### Proof-of-Work Requirements

All 29000 wrapper events must include proof-of-work per [NIP-13](https://github.com/nostr-protocol/nips/blob/master/13.md):

- **Required difficulty**: 16 (number of leading zero bits)
- **Committed difficulty**: Must be >= 16 (higher difficulties are accepted)
- **Validation**: Renoters check `NIP13.CommittedDifficulty(event) >= 16` before processing
- **Purpose**: Spam prevention and rate limiting

### Size Limits

- Maximum inner 29000 event size (before padding): 32KB (32768 bytes)
- Standardized 29001 container size (after encryption): Variable, but inner padded 29000 must be exactly 32KB
- Events exceeding 32KB before padding must be rejected by the client

## Rationale

### Why Ephemeral Events (29000/29001)?

Ephemeral events are non-persistent, preventing wrapper events from cluttering relay storage. Since wrapper events are intermediate routing artifacts, they don't need to be stored long-term.

### Why Nested Encryption?

Nested encryption provides true onion-routing where each Renoter only knows:
- The previous Renoter (from the sender's pubkey in 29001 events)
- The next Renoter (from the `p` tag after decryption)
- Nothing about the original sender or final destination

### Why Standardized Sizes (32KB)?

Fixed-size messages prevent metadata leakage through size analysis. All messages circulating between Renoters appear the same size, preventing traffic analysis.

### Why Proof-of-Work?

PoW prevents spam attacks on the Renoter network. The computational cost (difficulty 16 ≈ 65K attempts on average) makes spam economically unfeasible while remaining practical for legitimate use.

### Why Random Keys Per Layer?

Each wrapper layer uses a randomly generated key pair, breaking any linkability between layers. This ensures that even if one layer is compromised, other layers remain secure.

## Security Considerations

### Replay Attacks

Renoters MUST reject events that have already been processed. To implement this, Renoters must maintain a record of processed event IDs. Implementations should:

- Track processed event IDs for events within a reasonable time window
- Reject events with IDs that have been seen before
- Implement cache management to prevent unbounded memory growth

### Metadata Privacy

- **Size Analysis**: Standardized 32KB padding prevents size-based traffic analysis
- **Timing Analysis**: Random path shuffling and multi-relay publishing mitigate timing attacks
- **Relay Correlation**: Using multiple relays per Renoter prevents single-point correlation

### Key Management

- Renoter private keys must be kept secure
- Client wrapper keys are ephemeral and regenerated for each event
- Never reuse wrapper keys across events

### Network Privacy

- Clients should shuffle Renoter paths randomly to prevent consistent routing patterns
- Multiple relays should be used for redundancy and to reduce correlation risk

## Backwards Compatibility

This NIP is fully backwards compatible:

- Uses ephemeral event kinds (29000/29001) that don't interfere with existing events
- No changes to existing event structures
- Relays that don't implement this NIP will simply not store ephemeral wrapper events (per NIP-16)
- Clients and relays that don't support Renoter continue to function normally

## Reference Implementation

- **Go Implementation**: https://github.com/girino/renoter
- **Version**: 1.0.0-alpha

## Example

### Client Wrapping

Given a Renoter path: `[R1, R2, R3]` and original event `E`:

1. Wrap for R3: `E` → encrypted with R3's pubkey → `W3` (kind 29000)
2. Wrap for R2: `W3` → encrypted with R2's pubkey → `W2` (kind 29000)
3. Wrap for R1: `W2` → encrypted with R1's pubkey → `W1` (kind 29000)
4. Pad `W1` to 32KB → encrypt with R1's pubkey → `SW1` (kind 29001)
5. Publish `SW1` to relays

### Renoter Processing

When R1 receives `SW1`:
1. Decrypt → get padded `W1`
2. Remove padding → get `W1`
3. Verify PoW and replay protection
4. Decrypt `W1` → get `W2`
5. Pad `W2` to 32KB → encrypt with R2's pubkey → `SW2` (kind 29001)
6. Publish `SW2` to relays

When R2 receives `SW2`:
1. Decrypt → get padded `W2`
2. Remove padding → get `W2`
3. Verify PoW and replay protection
4. Decrypt `W2` → get `W3`
5. Pad `W3` to 32KB → encrypt with R3's pubkey → `SW3` (kind 29001)
6. Publish `SW3` to relays

When R3 receives `SW3`:
1. Decrypt → get padded `W3`
2. Remove padding → get `W3`
3. Verify PoW and replay protection
4. Decrypt `W3` → get original event `E`
5. Publish `E` to relays (final destination)

## Appendix: Event Flow Diagram

```
Client → [R1] → [R2] → [R3] → Final Relay
        SW1     SW2     SW3      E
```

Where:
- `SW1` = 29001 event encrypted for R1 (contains padded 29000 `W1`)
- `SW2` = 29001 event encrypted for R2 (contains padded 29000 `W2`)
- `SW3` = 29001 event encrypted for R3 (contains padded 29000 `W3`)
- `E` = Final event published to relay

