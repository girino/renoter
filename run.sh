#!/bin/bash

# Script to run 3 Renoter servers and 1 client from .env configuration
# Usage: ./run.sh
#
# Required .env variables:
#   RENOTER_RELAYS - Comma-separated relay URLs (e.g., wss://relay1.com,wss://relay2.com)
#   RENOTER_PATH - Comma-separated Renoter npubs (e.g., npub1...,npub2...,npub3...)
#   CLIENT_SERVER_RELAYS - Comma-separated relay URLs for client to send wrapped events
#
# Optional .env variables:
#   RENOTER_PRIVATE_KEY_1 - Private key for Renoter 1 in hex (leave empty to generate new)
#   RENOTER_PRIVATE_KEY_2 - Private key for Renoter 2 in hex (leave empty to generate new)
#   RENOTER_PRIVATE_KEY_3 - Private key for Renoter 3 in hex (leave empty to generate new)
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
    # if [ -f example.env ]; then
    #     echo "Copy example.env to .env and edit it:"
    #     echo "  cp example.env .env"
    # else
        echo "Please create a .env file with the following variables:"
        echo "  RENOTER_RELAYS=wss://relay1.com,wss://relay2.com"
        echo "  RENOTER_PATH=npub1...,npub2..."
        echo "  CLIENT_SERVER_RELAYS=wss://relay1.com,wss://relay2.com"
        echo "  CLIENT_LISTEN=:8080"
        echo "  VERBOSE="
    # fi
    exit 1
fi

# Source .env file
echo -e "${GREEN}Loading configuration from .env...${NC}"
set -a
source .env
set +a

# Check required variables
if [ -z "$RENOTER_PRIVATE_KEY_1" ]; then
    echo -e "${YELLOW}Warning: RENOTER_PRIVATE_KEY_1 not set, Renoter 1 will generate a new key${NC}"
fi
if [ -z "$RENOTER_PRIVATE_KEY_2" ]; then
    echo -e "${YELLOW}Warning: RENOTER_PRIVATE_KEY_2 not set, Renoter 2 will generate a new key${NC}"
fi
if [ -z "$RENOTER_PRIVATE_KEY_3" ]; then
    echo -e "${YELLOW}Warning: RENOTER_PRIVATE_KEY_3 not set, Renoter 3 will generate a new key${NC}"
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

# Clean and build executables
echo -e "${GREEN}Cleaning previous builds...${NC}"
go clean -i ./cmd/client ./cmd/server 2>/dev/null || true
rm -f renoter-client renoter-server 2>/dev/null || true

echo -e "${GREEN}Building executables...${NC}"
if ! go build -o renoter-client ./cmd/client; then
    echo -e "${RED}Error: Failed to build renoter-client${NC}"
    exit 1
fi
if ! go build -o renoter-server ./cmd/server; then
    echo -e "${RED}Error: Failed to build renoter-server${NC}"
    exit 1
fi
echo -e "${GREEN}Build successful${NC}"

# Cleanup function to kill all processes
cleanup() {
    echo -e "\n${YELLOW}Shutting down...${NC}"
    if [ ! -z "$SERVER_PID_1" ]; then
        echo "Killing Renoter 1 (PID: $SERVER_PID_1)"
        kill $SERVER_PID_1 2>/dev/null || true
    fi
    if [ ! -z "$SERVER_PID_2" ]; then
        echo "Killing Renoter 2 (PID: $SERVER_PID_2)"
        kill $SERVER_PID_2 2>/dev/null || true
    fi
    if [ ! -z "$SERVER_PID_3" ]; then
        echo "Killing Renoter 3 (PID: $SERVER_PID_3)"
        kill $SERVER_PID_3 2>/dev/null || true
    fi
    if [ ! -z "$CLIENT_PID" ]; then
        echo "Killing client (PID: $CLIENT_PID)"
        kill $CLIENT_PID 2>/dev/null || true
    fi
    wait $SERVER_PID_1 2>/dev/null || true
    wait $SERVER_PID_2 2>/dev/null || true
    wait $SERVER_PID_3 2>/dev/null || true
    wait $CLIENT_PID 2>/dev/null || true
    echo -e "${GREEN}Shutdown complete${NC}"
    exit 0
}

# Trap SIGINT and SIGTERM
trap cleanup SIGINT SIGTERM

# Start Renoter 1 using built executable
echo -e "${GREEN}Starting Renoter 1...${NC}"
if [ -z "$RENOTER_PRIVATE_KEY_1" ]; then
    ./renoter-server -relays="$RENOTER_RELAYS" > server1.log 2>&1 &
else
    ./renoter-server -private-key="$RENOTER_PRIVATE_KEY_1" -relays="$RENOTER_RELAYS" > server1.log 2>&1 &
fi
SERVER_PID_1=$!
echo "Renoter 1 PID: $SERVER_PID_1"
echo "Renoter 1 logs: server1.log"

# Start Renoter 2
echo -e "${GREEN}Starting Renoter 2...${NC}"
if [ -z "$RENOTER_PRIVATE_KEY_2" ]; then
    ./renoter-server -relays="$RENOTER_RELAYS" > server2.log 2>&1 &
else
    ./renoter-server -private-key="$RENOTER_PRIVATE_KEY_2" -relays="$RENOTER_RELAYS" > server2.log 2>&1 &
fi
SERVER_PID_2=$!
echo "Renoter 2 PID: $SERVER_PID_2"
echo "Renoter 2 logs: server2.log"

# Start Renoter 3
echo -e "${GREEN}Starting Renoter 3...${NC}"
if [ -z "$RENOTER_PRIVATE_KEY_3" ]; then
    ./renoter-server -relays="$RENOTER_RELAYS" > server3.log 2>&1 &
else
    ./renoter-server -private-key="$RENOTER_PRIVATE_KEY_3" -relays="$RENOTER_RELAYS" > server3.log 2>&1 &
fi
SERVER_PID_3=$!
echo "Renoter 3 PID: $SERVER_PID_3"
echo "Renoter 3 logs: server3.log"

# Wait a moment for servers to start
sleep 2

# Check if all servers are still running
if ! kill -0 $SERVER_PID_1 2>/dev/null; then
    echo -e "${RED}Error: Renoter 1 failed to start. Check server1.log for details.${NC}"
    cleanup
    exit 1
fi
if ! kill -0 $SERVER_PID_2 2>/dev/null; then
    echo -e "${RED}Error: Renoter 2 failed to start. Check server2.log for details.${NC}"
    cleanup
    exit 1
fi
if ! kill -0 $SERVER_PID_3 2>/dev/null; then
    echo -e "${RED}Error: Renoter 3 failed to start. Check server3.log for details.${NC}"
    cleanup
    exit 1
fi

# Start client using built executable
echo -e "${GREEN}Starting Renoter client...${NC}"
if [ -z "$VERBOSE" ]; then
    ./renoter-client -listen="$CLIENT_LISTEN" -path="$RENOTER_PATH" -server-relays="$CLIENT_SERVER_RELAYS" > client.log 2>&1 &
else
    VERBOSE="$VERBOSE" ./renoter-client -listen="$CLIENT_LISTEN" -path="$RENOTER_PATH" -server-relays="$CLIENT_SERVER_RELAYS" -verbose="$VERBOSE" > client.log 2>&1 &
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

echo -e "${GREEN}All processes started successfully!${NC}"
echo ""
echo "Renoter 1 PID: $SERVER_PID_1"
echo "Renoter 2 PID: $SERVER_PID_2"
echo "Renoter 3 PID: $SERVER_PID_3"
echo "Client PID: $CLIENT_PID"
echo ""
echo "Logs:"
echo "  Renoter 1: tail -f server1.log"
echo "  Renoter 2: tail -f server2.log"
echo "  Renoter 3: tail -f server3.log"
echo "  Client: tail -f client.log"
echo ""
echo -e "${YELLOW}Press Ctrl+C to stop all processes...${NC}"

# Wait for all processes and check their status periodically
while kill -0 $SERVER_PID_1 2>/dev/null || kill -0 $SERVER_PID_2 2>/dev/null || kill -0 $SERVER_PID_3 2>/dev/null || kill -0 $CLIENT_PID 2>/dev/null; do
    sleep 1
    # Check if any process died unexpectedly
    if ! kill -0 $SERVER_PID_1 2>/dev/null && [ ! -z "$SERVER_PID_1" ]; then
        echo -e "${RED}Renoter 1 process died unexpectedly!${NC}"
        cleanup
        exit 1
    fi
    if ! kill -0 $SERVER_PID_2 2>/dev/null && [ ! -z "$SERVER_PID_2" ]; then
        echo -e "${RED}Renoter 2 process died unexpectedly!${NC}"
        cleanup
        exit 1
    fi
    if ! kill -0 $SERVER_PID_3 2>/dev/null && [ ! -z "$SERVER_PID_3" ]; then
        echo -e "${RED}Renoter 3 process died unexpectedly!${NC}"
        cleanup
        exit 1
    fi
    if ! kill -0 $CLIENT_PID 2>/dev/null && [ ! -z "$CLIENT_PID" ]; then
        echo -e "${RED}Client process died unexpectedly!${NC}"
        cleanup
        exit 1
    fi
done

# All processes exited naturally
echo -e "${GREEN}All processes have exited${NC}"

