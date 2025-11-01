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
  -e RENOTER_PATH="npub1...,npub2..." \
  -e CLIENT_SERVER_RELAYS="wss://relay1.com,wss://relay2.com" \
  ghcr.io/${{ github.repository }}/renoter-client:${{ version }}
```

**Server:**
```bash
docker pull ghcr.io/${{ github.repository }}/renoter-server:${{ version }}
docker run -d \
  -e RENOTER_PRIVATE_KEY="your-private-key-hex" \
  -e RENOTER_RELAYS="wss://relay1.com,wss://relay2.com" \
  ghcr.io/${{ github.repository }}/renoter-server:${{ version }}
```

### Docker Compose

See the [README](https://github.com/${{ github.repository }}/blob/main/README.md#docker-deployment) for Docker Compose usage.

## Documentation

Full documentation is available in the [README](https://github.com/${{ github.repository }}/blob/main/README.md).

## Checksums

See `checksums.txt` in the assets for SHA256 checksums of all binaries.
