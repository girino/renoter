package config

import "testing"

func TestWrapperEventKind(t *testing.T) {
	// Verify that WrapperEventKind is defined and has the expected value
	if WrapperEventKind != 29000 {
		t.Errorf("WrapperEventKind = %d, want 29000", WrapperEventKind)
	}

	// Verify it's within the ephemeral event range (20000-29999)
	if WrapperEventKind < 20000 || WrapperEventKind > 29999 {
		t.Errorf("WrapperEventKind = %d, should be in ephemeral event range (20000-29999)", WrapperEventKind)
	}
}

