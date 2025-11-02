package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/fiatjaf/khatru"
)

// TestRelay represents a local relay server for testing
type TestRelay struct {
	relay  *khatru.Relay
	server *http.Server
	port   int
	url    string
}

// StartTestRelay starts a local khatru relay on a random available port
func StartTestRelay(ctx context.Context) (*TestRelay, error) {
	// Create a new khatru relay
	relay := khatru.NewRelay()

	// Find an available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("failed to find available port: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("ws://localhost:%d", port)

	// Create HTTP server
	server := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: relay,
	}

	// Start server in goroutine
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			// Log error if needed (can't use logging here as it might not be initialized)
			// The test will detect if the relay is not working
		}
	}()

	testRelay := &TestRelay{
		relay:  relay,
		server: server,
		port:   port,
		url:    url,
	}

	return testRelay, nil
}

// URL returns the WebSocket URL for this relay
func (tr *TestRelay) URL() string {
	return tr.url
}

// Relay returns the underlying khatru relay instance
func (tr *TestRelay) Relay() *khatru.Relay {
	return tr.relay
}

// Stop shuts down the test relay server
func (tr *TestRelay) Stop(ctx context.Context) error {
	if tr.server != nil {
		return tr.server.Shutdown(ctx)
	}
	return nil
}
