package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
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
		privateKey = flag.String("private-key", "", "Private key in hex format (or leave empty to generate new)")
		relays     = flag.String("relays", "", "Comma-separated relay URLs for listening and forwarding (e.g., wss://relay1.com,wss://relay2.com)")
		configFile = flag.String("config", "", "Path to config file (not implemented yet)")
		verbose    = flag.String("verbose", "", "Verbose logging (true/all, or comma-separated module.method filters)")
	)
	flag.Parse()

	// Override with flag if provided
	if *verbose != "" {
		logging.SetVerbose(*verbose)
	}

	if *relays == "" {
		log.Fatal("Error: -relays is required (comma-separated relay URLs)")
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

	// Parse relay URLs
	relayList := strings.Split(*relays, ",")
	for i := range relayList {
		relayList[i] = strings.TrimSpace(relayList[i])
		if relayList[i] == "" {
			log.Fatalf("Error: empty relay URL at index %d", i)
		}
	}

	log.Printf("Using %d relays: %v", len(relayList), relayList)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create Renoter instance with SimplePool
	renoter, err := server.NewRenoter(ctx, sk, relayList)
	if err != nil {
		log.Fatalf("Error: failed to create Renoter: %v", err)
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
		os.Exit(0)
	}()

	// Subscribe to wrapped events on all relays
	log.Printf("Subscribing to %d relays for wrapped events", len(relayList))
	log.Println("Press Ctrl+C to stop")

	err = renoter.SubscribeToWrappedEvents(ctx)
	if err != nil {
		log.Fatalf("Error: failed to subscribe to wrapped events: %v", err)
	}

	// Keep running
	<-ctx.Done()
}

