#!/bin/bash

# Script to run both Renoter client and server from .env configuration
# Usage: ./run.sh
#
# Required .env variables:
#   RENOTER_RELAYS - Comma-separated relay URLs (e.g., wss://relay1.com,wss://relay2.com)
#   RENOTER_PATH - Comma-separated Renoter npubs (e.g., npub1...,npub2...)
#   CLIENT_SERVER_RELAYS - Comma-separated relay URLs for client to send wrapped events
#
# Optional .env variables:
#   RENOTER_PRIVATE_KEY - Private key in hex (leave empty to generate new)
#   CLIENT_LISTEN - Client listen address (default: :8080)
#   VERBOSE - Debug logging (true/all or module.method filters)

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if .env file exists
if [ ! -f .env ]; then
    echo -e "${RED}Error: .env file not found${NC}"
    echo ""
    if [ -f example.env ]; then
        echo "Copy example.env to .env and edit it:"
        echo "  cp example.env .env"
    else
        echo "Please create a .env file with the following variables:"
        echo "  RENOTER_RELAYS=wss://relay1.com,wss://relay2.com"
        echo "  RENOTER_PATH=npub1...,npub2..."
        echo "  CLIENT_SERVER_RELAYS=wss://relay1.com,wss://relay2.com"
        echo "  CLIENT_LISTEN=:8080"
        echo "  VERBOSE="
    fi
    exit 1
fi

# Source .env file
echo -e "${GREEN}Loading configuration from .env...${NC}"
set -a
source .env
set +a

# Check required variables
if [ -z "$RENOTER_PRIVATE_KEY" ]; then
    echo -e "${YELLOW}Warning: RENOTER_PRIVATE_KEY not set, server will generate a new one${NC}"
fi

if [ -z "$RENOTER_RELAYS" ]; then
    echo -e "${RED}Error: RENOTER_RELAYS is required in .env${NC}"
    exit 1
fi

if [ -z "$RENOTER_PATH" ]; then
    echo -e "${RED}Error: RENOTER_PATH is required in .env${NC}"
    exit 1
fi

if [ -z "$CLIENT_SERVER_RELAYS" ]; then
    echo -e "${RED}Error: CLIENT_SERVER_RELAYS is required in .env${NC}"
    exit 1
fi

# Set defaults
CLIENT_LISTEN=${CLIENT_LISTEN:-":8080"}
VERBOSE=${VERBOSE:-""}

# Verify Go code compiles
echo -e "${GREEN}Verifying Go code compiles...${NC}"
if ! go build ./cmd/client ./cmd/server > /dev/null 2>&1; then
    echo -e "${RED}Error: Go code does not compile. Fix errors before running.${NC}"
    exit 1
fi

# Cleanup function to kill both processes
cleanup() {
    echo -e "\n${YELLOW}Shutting down...${NC}"
    if [ ! -z "$SERVER_PID" ]; then
        echo "Killing server (PID: $SERVER_PID)"
        kill $SERVER_PID 2>/dev/null || true
    fi
    if [ ! -z "$CLIENT_PID" ]; then
        echo "Killing client (PID: $CLIENT_PID)"
        kill $CLIENT_PID 2>/dev/null || true
    fi
    wait $SERVER_PID 2>/dev/null || true
    wait $CLIENT_PID 2>/dev/null || true
    echo -e "${GREEN}Shutdown complete${NC}"
    exit 0
}

# Trap SIGINT and SIGTERM
trap cleanup SIGINT SIGTERM

# Start server using go run (always uses latest code)
echo -e "${GREEN}Starting Renoter server (go run)...${NC}"
if [ -z "$RENOTER_PRIVATE_KEY" ]; then
    go run ./cmd/server -relays="$RENOTER_RELAYS" > server.log 2>&1 &
else
    go run ./cmd/server -private-key="$RENOTER_PRIVATE_KEY" -relays="$RENOTER_RELAYS" > server.log 2>&1 &
fi
SERVER_PID=$!
echo "Server PID: $SERVER_PID"
echo "Server logs: server.log"

# Wait a moment for server to start
sleep 1

# Check if server is still running
if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo -e "${RED}Error: Server failed to start. Check server.log for details.${NC}"
    exit 1
fi

# Start client using go run (always uses latest code)
echo -e "${GREEN}Starting Renoter client (go run)...${NC}"
if [ -z "$VERBOSE" ]; then
    go run ./cmd/client -listen="$CLIENT_LISTEN" -path="$RENOTER_PATH" -server-relays="$CLIENT_SERVER_RELAYS" > client.log 2>&1 &
else
    VERBOSE="$VERBOSE" go run ./cmd/client -listen="$CLIENT_LISTEN" -path="$RENOTER_PATH" -server-relays="$CLIENT_SERVER_RELAYS" -verbose="$VERBOSE" > client.log 2>&1 &
fi
CLIENT_PID=$!
echo "Client PID: $CLIENT_PID"
echo "Client logs: client.log"

# Wait a moment for client to start
sleep 1

# Check if client is still running
if ! kill -0 $CLIENT_PID 2>/dev/null; then
    echo -e "${RED}Error: Client failed to start. Check client.log for details.${NC}"
    cleanup
    exit 1
fi

echo -e "${GREEN}Both processes started successfully!${NC}"
echo ""
echo "Server PID: $SERVER_PID"
echo "Client PID: $CLIENT_PID"
echo ""
echo "Logs:"
echo "  Server: tail -f server.log"
echo "  Client: tail -f client.log"
echo ""
echo -e "${YELLOW}Press Ctrl+C to stop both processes...${NC}"

# Wait for both processes and check their status periodically
while kill -0 $SERVER_PID 2>/dev/null || kill -0 $CLIENT_PID 2>/dev/null; do
    sleep 1
    # Check if either process died unexpectedly
    if ! kill -0 $SERVER_PID 2>/dev/null && [ ! -z "$SERVER_PID" ]; then
        echo -e "${RED}Server process died unexpectedly!${NC}"
        cleanup
        exit 1
    fi
    if ! kill -0 $CLIENT_PID 2>/dev/null && [ ! -z "$CLIENT_PID" ]; then
        echo -e "${RED}Client process died unexpectedly!${NC}"
        cleanup
        exit 1
    fi
done

# Both processes exited naturally
echo -e "${GREEN}Both processes have exited${NC}"

