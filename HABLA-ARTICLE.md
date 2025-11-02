# From Proposal to Implementation: What Changed in Nostr Renoter v1.0.0

> **Reference:** This article compares the implementation with my [original Renoter proposal on Habla News](https://habla.news/a/naddr1qvzqqqr4gupzq0l6cwnvsk0242xdmkevwqp2dcgtx0h7hyksykc5att03gkk2ejhqqxnzde5xqenxwfj8yunzd3s3t9en6).

## Introduction

Earlier this year, I published a [proposal for "Renoters"](https://habla.news/a/naddr1qvzqqqr4gupzq0l6cwnvsk0242xdmkevwqp2dcgtx0h7hyksykc5att03gkk2ejhqqxnzde5xqenxwfj8yunzd3s3t9en6)—an onion-routing system for Nostr inspired by Mixminion remailers. That proposal envisioned nested encryption with Cashu token incentives, expiration tags, and specific size constraints. Now, with the release of Renoter v1.0.0, I've delivered a production-ready implementation that takes a practical approach while maintaining the core privacy goals.

This article describes what was actually built and how it differs from my [original proposal](https://habla.news/a/naddr1qvzqqqr4gupzq0l6cwnvsk0242xdmkevwqp2dcgtx0h7hyksykc5att03gkk2ejhqqxnzde5xqenxwfj8yunzd3s3t9en6), highlighting both pragmatic trade-offs I made during implementation and novel additions that enhance security.

## Core Concept: What Stayed the Same

The fundamental onion-routing mechanism remains unchanged:

- **Nested Encryption**: Events are wrapped in reverse order, with each Renoter decrypting one layer and forwarding to the next
- **Random Keys Per Layer**: Each wrapper layer uses a freshly generated key pair, breaking linkability
- **Ephemeral Events**: Wrapper events use ephemeral kinds (29000/29001) that don't persist on relays
- **Padding Mechanism**: Messages are padded to standardized sizes to prevent metadata leakage through size analysis

## What Changed: Encryption Method

**Original Proposal:**
- Asymmetric encryption using "Nostr's standard encryption mechanism"
- Encryption via shared secrets derived from the sender's private key and recipient's public key

**Implementation:**
- **NIP-44 encryption** (symmetric shared secret encryption)
- Each wrapper generates a random private key and uses NIP-44's `GenerateConversationKey()` to derive the conversation key from the Renoter's public key

**Why the Change?**
NIP-44 provides a standardized, battle-tested encryption scheme that's well-integrated into the Nostr ecosystem. The symmetric nature simplifies key management while maintaining security properties needed for onion-routing.

## What Changed: Standardized Message Sizes

**Original Proposal:**
- Fixed message size: **1023 bytes**
- Padding via `["padding", "randomstring"]` tag format
- Rationale: Staying under common relay 1KB size limits

**Implementation:**
- Fixed message size: **32KB (32768 bytes)**
- Padding via `["padding", "<hex-string>"]` tags, where the hex string length is calculated to make the serialized event exactly 32KB

**Why the Change?**
Modern Nostr relays typically support much larger event sizes. A 32KB limit allows for more complex nested routing paths (multiple Renoters) and larger original events while still preventing size-based traffic analysis. The padding mechanism ensures all messages between Renoters appear identical in size.

## What Changed: Spam Protection

**Original Proposal:**
- No explicit spam protection mechanism
- Relied on Cashu token payments to discourage spam (see below)

**Implementation:**
- **Proof-of-Work (NIP-13)** with difficulty 16 required for all 29000 wrapper events
- Each wrapper event must include a `["nonce", "<nonce>", "<committed-difficulty>"]` tag
- Average computational cost: ~65,536 attempts per wrapper event

**Why the Addition?**
PoW provides a decentralized, Sybil-resistant spam prevention mechanism that doesn't require payment infrastructure. It makes spam economically unfeasible while remaining practical for legitimate use. The difficulty can be adjusted based on network conditions.

## What Changed: Event Structure

**Original Proposal:**
- Single wrapper event type with encrypted content and routing information ("p" tag)

**Implementation:**
- **Two-layer structure:**
  - **Kind 29000**: Inner wrapper events with PoW, containing encrypted inner events
  - **Kind 29001**: Standardized container events containing padded 29000 events encrypted for the first Renoter

This structure separates routing logic (29000) from standardized packaging (29001), allowing for better size control and cleaner protocol separation.

## What Changed: Replay Attack Protection

**Original Proposal:**
- Store both event IDs and decrypted events
- Retention period: "reasonable period"

**Implementation:**
- **In-memory cache** storing only event IDs (bounded to 5,000 entries)
- Automatic pruning when cache reaches capacity
- Age validation: events older than a configurable cutoff (default: 2 hours) are rejected

**Why the Simplification?**
Storing only IDs reduces memory footprint while maintaining replay protection. The bounded cache prevents unbounded growth. Age validation complements replay protection by rejecting stale events even if they're new to the cache.

## What Wasn't Implemented: Cashu Token Incentives

**Original Proposal:**
- Cashu tokens embedded in wrapped events as payment
- Renoters decrypt and redeem tokens
- Paid Renoters prioritize or require tokens
- Tiers based on token denominations

**Implementation:**
- **Not implemented**

**Why Not?**
Cashu integration adds significant complexity and requires infrastructure for token minting, redemption, and relay payment verification. For an initial release, PoW-based spam protection was chosen as a simpler, more decentralized alternative. Future versions may explore payment mechanisms as the network matures.

## What Wasn't Implemented: NIP-40 Expiration Tags

**Original Proposal:**
- Short expiration times (< 5 minutes) via NIP-40 `expiration` tag
- Rationale: Make intercepted events invalid quickly

**Implementation:**
- **Not implemented**
- Relies on event age validation instead

**Why Not?**
Age validation at Renoter servers provides similar protection without requiring relay support for NIP-40. Since wrapper events are ephemeral (non-persistent), expiration tags provide less benefit than for persistent events.

## What Wasn't Implemented: NIP-70 Protected Events

**Original Proposal:**
- Re-wrap decrypted events as NIP-70 protected events before forwarding
- Rationale: Prevent replay attacks on intermediate events

**Implementation:**
- **Not implemented**
- Uses event ID tracking and age validation instead

**Why Not?**
NIP-70 adds complexity and requires relay support. The implemented replay protection (ID cache + age validation) provides effective protection without additional protocol dependencies.

## What Wasn't Implemented: Traffic Analysis Mitigations

**Original Proposal:**
- Random delays in event relaying
- Dummy packets to complicate statistical analysis
- Tor integration for additional anonymity

**Implementation:**
- **Not implemented**
- Multi-relay publishing provides some correlation resistance

**Why Not?**
These mitigations require network-level analysis and careful timing to be effective. They also add latency and complexity. Multi-relay support helps by distributing events across multiple relays, making correlation harder. Future versions may explore these techniques based on real-world threat analysis.

## What Was Added: Multi-Relay Support

**Original Proposal:**
- Not explicitly mentioned

**Implementation:**
- **Multi-relay redundancy** using `nostr.SimplePool`
- Clients and servers can specify multiple relays for publishing and subscribing
- Automatic failover if one relay is unavailable

**Why the Addition?**
Multi-relay support improves reliability and makes traffic analysis more difficult. Publishing to multiple relays reduces correlation risk and provides redundancy against relay failures.

## What Was Added: Comprehensive Validation

**Implementation adds:**
- Path validation (no duplicates, valid npub format)
- Event age validation
- Proof-of-work validation at multiple layers
- Signature verification at each step
- Event ID integrity checks after padding removal

These validations ensure protocol correctness and prevent various attack vectors that I didn't explicitly address in my [original proposal](https://habla.news/a/naddr1qvzqqqr4gupzq0l6cwnvsk0242xdmkevwqp2dcgtx0h7hyksykc5att03gkk2ejhqqxnzde5xqenxwfj8yunzd3s3t9en6).

## Practical Impact

The implemented version prioritizes:

1. **Simplicity**: Easier to deploy and maintain without payment infrastructure
2. **Decentralization**: PoW doesn't require trusted payment providers
3. **Standardization**: Uses well-established NIPs (NIP-44, NIP-13) that are widely supported
4. **Pragmatism**: Focuses on core privacy goals (metadata hiding, anonymous routing) while deferring advanced mitigations

## Security Considerations

The implementation maintains strong privacy properties:
- **Metadata Privacy**: Standardized sizes and nested encryption hide sender/receiver information
- **Anonymous Routing**: Each Renoter only knows previous and next hops
- **Spam Resistance**: PoW makes spam costly
- **Replay Protection**: ID cache and age validation prevent replay attacks

However, some advanced attack vectors (timing analysis, correlation attacks) aren't explicitly mitigated. These may be addressed in future versions based on real-world usage and threat analysis.

## Looking Forward

My [original proposal](https://habla.news/a/naddr1qvzqqqr4gupzq0l6cwnvsk0242xdmkevwqp2dcgtx0h7hyksykc5att03gkk2ejhqqxnzde5xqenxwfj8yunzd3s3t9en6) included many forward-looking features that remain valuable but were deferred for the initial release. Future versions may explore:

- Payment mechanisms (Cashu or other)
- Advanced traffic analysis mitigations
- NIP-40/NIP-70 integration where beneficial
- Network-wide statistics and monitoring
- Renoter reputation systems

## Conclusion

Renoter v1.0.0 implements a production-ready onion-routing system that achieves the core privacy goals of my [original proposal](https://habla.news/a/naddr1qvzqqqr4gupzq0l6cwnvsk0242xdmkevwqp2dcgtx0h7hyksykc5att03gkk2ejhqqxnzde5xqenxwfj8yunzd3s3t9en6) through a pragmatic, standards-based approach. While some features I originally proposed were deferred, the implementation adds its own innovations (PoW, two-layer structure, multi-relay support) that enhance security and usability.

The focus on simplicity and standardization makes Renoter easier to deploy and integrate, which is crucial for adoption. As the network grows and usage patterns emerge, future versions can incorporate the more advanced features from my [original proposal](https://habla.news/a/naddr1qvzqqqr4gupzq0l6cwnvsk0242xdmkevwqp2dcgtx0h7hyksykc5att03gkk2ejhqqxnzde5xqenxwfj8yunzd3s3t9en6).

**Try Renoter v1.0.0:**
- GitHub: https://github.com/girino/renoter
- Release: https://github.com/girino/renoter/releases/tag/v1.0.0
- NIP Draft: https://github.com/girino/renoter/blob/main/NIP-RENOTER.md

---

*This article compares my [original Renoter proposal](https://habla.news/a/naddr1qvzqqqr4gupzq0l6cwnvsk0242xdmkevwqp2dcgtx0h7hyksykc5att03gkk2ejhqqxnzde5xqenxwfj8yunzd3s3t9en6) with the v1.0.0 implementation. For technical details, see the NIP draft and source code.*

