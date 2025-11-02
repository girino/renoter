package config

// WrapperEventKind is the ephemeral event kind used for inner wrapper events (routing layer).
// Ephemeral events (20000-29999) are non-persistent and won't be stored by relays.
const WrapperEventKind = 29000

// StandardizedWrapperKind is the ephemeral event kind used for outer standardized size containers.
// These events are always padded to exactly StandardizedSize (8KB) to hide message size metadata.
const StandardizedWrapperKind = 29001

// StandardizedSize is the target size for standardized wrapper events (32KB).
const StandardizedSize = 32 * 1024 // 32768 bytes

// PoWDifficulty is the proof-of-work difficulty for 29000 wrapper events (number of leading zero bits required).
// Default is 20, which requires ~1048576 attempts on average. This can be adjusted to balance spam prevention vs CPU cost.
const PoWDifficulty = 20
