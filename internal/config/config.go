package config

// WrapperEventKind is the ephemeral event kind used for inner wrapper events (routing layer).
// Ephemeral events (20000-29999) are non-persistent and won't be stored by relays.
const WrapperEventKind = 29000

// StandardizedWrapperKind is the ephemeral event kind used for outer standardized size containers.
// These events are always padded to exactly StandardizedSize (4KB) to hide message size metadata.
const StandardizedWrapperKind = 29001

// StandardizedSize is the target size for standardized wrapper events (4KB).
const StandardizedSize = 4 * 1024 // 4096 bytes
