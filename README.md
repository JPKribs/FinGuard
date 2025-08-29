<div align="center">

<img src="branding/FinGuard.png" alt="FinGuard Logo" width="200"/>

**UserSpace WireGuard Service to Proxy Specific Traffic to Services**

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

[Purpose](#purpose) • [Quick Start](#quick-start) • [Configuration](#configuration)

</div>

## Purpose

This tool allows a local machine or SBC to act as a bridge to services behind WireGuard. This enables services to be be accessible in remote locations, without needing to put the entire network behind WireGuard or install WireGuard on every network device. Using Avahi, services are broadcast using friendly `service.local` syntax for ease of use. 

This project was originally created with Jellyfin in mind.

## Quick Start

### Prerequisites
- Go 1.24+ (for building from source)
- sudo privileges (for TUN device creation)

### Systemd Installation (Debian)

```bash
# Clone the repository
git clone https://github.com/JPKribs/FinGuard.git
cd FinGuard

# Run the build script
./packaging/debian/build-deb.sh

# Install the built file
sudo dpkg -i *deb file produced in build*

# Start the service
sudo systemctl start finguard
```

### First Run

1. Open the web interface: `http://localhost:10000`
2. Enter your admin token from `/etc/finguard/config.yaml`
3. Add services through the Services tab
4. Configure WireGuard tunnels through the Tunnels tab

## Configuration

### File Structure
```
- Main config: /etc/finguard/config.yaml
- Services:    /etc/finguard/services.yaml
- WireGuard:   /etc/finguard/wireguard.yaml
- Update:      /etc/finguard/update.yaml
```

### Main Configuration (`config.yaml`)
```yaml
server:
  http_addr: "0.0.0.0:10000"
  proxy_addr: "0.0.0.0:80"
  admin_token: "your-secure-token-here"

log:
  level: "info"

# External config files
services_file: "services.yaml"
wireguard_file: "wireguard.yaml"
update_file: "update.yaml"

# Enables Avahi & mDNS
discovery:
  enable: true
  mdns:
    enabled: true
```

## Usage Examples

### Adding Tunnels via Web Interface
Tunnels can be added through the web interface at `http://localhost:10000`. Each tunnel requires at least one peer. 

<img width="360" height="713" alt="Screenshot 2025-08-29 at 10 24 42" src="https://github.com/user-attachments/assets/622905c8-3c7b-47f3-b824-a6a12a92a55e" />

### Adding Tunnels via API
```bash
# Generate keys
wg genkey > private.key
wg pubkey < private.key > public.key

# Create tunnel via API
curl -X POST http://localhost:10000/api/v1/tunnels \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "homelab",
    "private_key": "'$(cat private.key)'",
    "addresses": ["10.0.0.1/24"],
    "mtu": 1420,
    "peers": [{
      "name": "server",
      "public_key": "SERVER_PUBLIC_KEY",
      "endpoint": "vpn.example.com:51820",
      "allowed_ips": ["10.100.0.0/24"],
      "persistent_keepalive": 25
    }]
  }'
```

### Adding Services via Web Interface
Services can be added through the web interface at `http://localhost:10000`. Each service creates a "subdomain" route in Avahi:

- `jellyfin.local` → `http://192.168.1.100:8096`
- `homeassistant.local` → `http://192.168.1.50:8123`
- Services marked as default will be used for any unmatched requests
- A Tunnel must be selected for traffic to route over the tunnel

<img width="795" height="652" alt="Screenshot 2025-08-29 at 10 25 22" src="https://github.com/user-attachments/assets/8a89666e-c66a-4bfa-b869-9d950c65ff82" />

### Adding Services via API
```bash
curl -X POST http://localhost:10000/api/v1/services \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "myapp",
    "upstream": "http://192.168.1.100:8080",
    "websocket": true,
    "publish_mdns": true
  }'
```

## License

MIT License - see [LICENSE](LICENSE) file for details.
