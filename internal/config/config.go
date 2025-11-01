package config

// WrapperEventKind is the ephemeral event kind used for wrapper events.
// Ephemeral events (20000-29999) are non-persistent and won't be stored by relays.
const WrapperEventKind = 29000
