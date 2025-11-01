package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"renoter/pkg/server"
	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func main() {
	// Initialize logging from environment variable
	logging.SetVerbose(os.Getenv("VERBOSE"))

	var (
		privateKey   = flag.String("private-key", "", "Private key in hex format (or leave empty to generate new)")
		listenRelay  = flag.String("listen-relay", "", "Relay URL to listen for wrapped events (e.g., wss://relay.example.com)")
		forwardRelay = flag.String("forward-relay", "", "Relay URL to forward decrypted events (e.g., wss://relay.example.com)")
		configFile   = flag.String("config", "", "Path to config file (not implemented yet)")
		verbose      = flag.String("verbose", "", "Verbose logging (true/all, or comma-separated module.method filters)")
	)
	flag.Parse()

	// Override with flag if provided
	if *verbose != "" {
		logging.SetVerbose(*verbose)
	}

	if *listenRelay == "" {
		log.Fatal("Error: -listen-relay is required")
	}
	if *forwardRelay == "" {
		log.Fatal("Error: -forward-relay is required")
	}

	// Ignore config file for now (future enhancement)
	if *configFile != "" {
		log.Println("Warning: -config flag is not yet implemented, ignoring")
	}

	// Generate or use provided private key
	sk := *privateKey
	if sk == "" {
		sk = nostr.GeneratePrivateKey()
		log.Println("Generated new private key")
	} else {
		log.Println("Using provided private key")
	}

	// Get public key
	pubkey, err := nostr.GetPublicKey(sk)
	if err != nil {
		log.Fatalf("Error: failed to get public key: %v", err)
	}

	// Encode as npub
	npub, err := nip19.EncodePublicKey(pubkey)
	if err != nil {
		log.Fatalf("Error: failed to encode npub: %v", err)
	}

	log.Printf("Renoter public key (npub): %s", npub)

	// Create Renoter instance
	renoter, err := server.NewRenoter(sk, *forwardRelay)
	if err != nil {
		log.Fatalf("Error: failed to create Renoter: %v", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
		os.Exit(0)
	}()

	// Subscribe to wrapped events
	log.Printf("Connecting to listen relay: %s", *listenRelay)
	log.Printf("Will forward to relay: %s", *forwardRelay)
	log.Println("Press Ctrl+C to stop")

	err = renoter.SubscribeToWrappedEvents(ctx, *listenRelay)
	if err != nil {
		log.Fatalf("Error: failed to subscribe to wrapped events: %v", err)
	}

	// Keep running
	<-ctx.Done()
}

