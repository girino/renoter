package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"renoter/pkg/client"
	"github.com/fiatjaf/khatru"
	"github.com/girino/nostr-lib/logging"
)

func main() {
	// Initialize logging from environment variable
	logging.SetVerbose(os.Getenv("VERBOSE"))

	var (
		listenAddr    = flag.String("listen", ":8080", "Address and port to listen on (e.g., :8080)")
		path          = flag.String("path", "", "Comma-separated list of Renoter npubs (e.g., npub1...,npub2...)")
		serverRelay   = flag.String("server-relay", "", "Relay URL where wrapped events will be sent (e.g., wss://relay.example.com)")
		configFile    = flag.String("config", "", "Path to config file (not implemented yet)")
		verbose       = flag.String("verbose", "", "Verbose logging (true/all, or comma-separated module.method filters)")
	)
	flag.Parse()

	// Override with flag if provided
	if *verbose != "" {
		logging.SetVerbose(*verbose)
	}

	if *path == "" {
		log.Fatal("Error: -path is required (comma-separated npubs)")
	}
	if *serverRelay == "" {
		log.Fatal("Error: -server-relay is required (relay URL for wrapped events)")
	}

	// Ignore config file for now (future enhancement)
	if *configFile != "" {
		log.Println("Warning: -config flag is not yet implemented, ignoring")
	}

	// Parse Renoter path
	npubs := strings.Split(*path, ",")
	for i := range npubs {
		npubs[i] = strings.TrimSpace(npubs[i])
	}

	// Validate path
	renterPath, err := client.ValidatePath(npubs)
	if err != nil {
		log.Fatalf("Error: invalid Renoter path: %v", err)
	}

	log.Printf("Validated Renoter path with %d nodes", len(renterPath))

	// Create khatru relay
	relay := khatru.NewRelay()

	// Setup relay to intercept and wrap events
	err = client.SetupRelay(relay, renterPath, *serverRelay)
	if err != nil {
		log.Fatalf("Error: failed to setup relay: %v", err)
	}

	// Setup HTTP handlers on router
	mux := relay.Router()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Nostr Renoter Client\n\n")
		fmt.Fprintf(w, "Connect your Nostr client to ws://%s\n", *listenAddr)
	})

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		os.Exit(0)
	}()

	// Parse listen address
	host, port := "", 8080
	if *listenAddr != "" {
		parts := strings.Split(*listenAddr, ":")
		if len(parts) == 2 {
			host = parts[0]
			if parts[1] != "" {
				fmt.Sscanf(parts[1], "%d", &port)
			}
		} else if len(parts) == 1 && parts[0] != "" {
			fmt.Sscanf(parts[0], "%d", &port)
		}
	}
	if host == "" {
		host = "0.0.0.0"
	}

	// Start server
	log.Printf("Starting Renoter client on %s:%d", host, port)
	log.Printf("Wrapping events and forwarding to %s", *serverRelay)
	log.Println("Press Ctrl+C to stop")

	if err := relay.Start(host, port); err != nil {
		log.Fatalf("Error: failed to start server: %v", err)
	}
}

