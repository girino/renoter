## Installation

### Using Binaries

Download the appropriate binary for your platform from the assets below.

**Client:**
```bash
# Linux
wget https://github.com/${{ github.repository }}/releases/download/${{ tag }}/renoter-client-linux-amd64
chmod +x renoter-client-linux-amd64
./renoter-client-linux-amd64 -path="npub1...,npub2..." -server-relays="wss://relay1.com,wss://relay2.com"

# macOS
wget https://github.com/${{ github.repository }}/releases/download/${{ tag }}/renoter-client-darwin-amd64
chmod +x renoter-client-darwin-amd64
./renoter-client-darwin-amd64 -path="npub1...,npub2..." -server-relays="wss://relay1.com,wss://relay2.com"

# Windows
# Download renoter-client-windows-amd64.exe and run it
```

**Server:**
```bash
# Linux
wget https://github.com/${{ github.repository }}/releases/download/${{ tag }}/renoter-server-linux-amd64
chmod +x renoter-server-linux-amd64
./renoter-server-linux-amd64 -relays="wss://relay1.com,wss://relay2.com"

# macOS
wget https://github.com/${{ github.repository }}/releases/download/${{ tag }}/renoter-server-darwin-amd64
chmod +x renoter-server-darwin-amd64
./renoter-server-darwin-amd64 -relays="wss://relay1.com,wss://relay2.com"

# Windows
# Download renoter-server-windows-amd64.exe and run it
```

### Using Docker

**Client:**
```bash
docker pull ghcr.io/${{ github.repository }}/renoter-client:${{ version }}
docker run -d \
  -p 8080:8080 \
  ghcr.io/${{ github.repository }}/renoter-client:${{ version }} \
  -listen=":8080" \
  -path="npub1...,npub2..." \
  -server-relays="wss://relay1.com,wss://relay2.com"
```

**Server:**
```bash
docker pull ghcr.io/${{ github.repository }}/renoter-server:${{ version }}
docker run -d \
  ghcr.io/${{ github.repository }}/renoter-server:${{ version }} \
  -private-key="your-private-key-hex" \
  -relays="wss://relay1.com,wss://relay2.com"
```

Note: Only `VERBOSE` can be set as an environment variable. All other parameters must be passed as command-line flags.

### Docker Compose

See the [README](https://github.com/${{ github.repository }}/blob/main/README.md#docker-deployment) for Docker Compose usage.

## Documentation

Full documentation is available in the [README](https://github.com/${{ github.repository }}/blob/main/README.md).

## Checksums

See `checksums.txt` in the assets for SHA256 checksums of all binaries.
