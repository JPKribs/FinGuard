# FinGuard

Minimal Debian SBC "boot & run" project to:

1. Install WireGuard (client).
2. Configure an NGINX reverse‑proxy for your defined services.
3. Advertise your Jellyfin server via mDNS using the Jellyfin Discovery Proxy.

---

## Goal and Architecture

FinGuard turns any small single-board computer (SBC) running Debian into a **dedicated WireGuard bridge** for your media ecosystem. Instead of installing a WireGuard client on every device or reconfiguring your router, you point all media clients at `http://<service>.local` and let FinGuard handle:

- **WireGuard**: Securely tunnel traffic from local devices into your remote network.
- **NGINX**: Create separate virtual hosts for each service with SSL termination.
- **mDNS Discovery**: Advertise your services automatically via Avahi and Jellyfin Discovery Proxy.

### Why a dedicated bridge?

- **Simplicity**: No per-device VPN configuration. Point all devices at individual `.local` addresses.
- **Compatibility**: Older devices or smart TVs often lack WireGuard support. The bridge handles encryption.
- **Isolation**: You can firewall or monitor the bridge separately, without affecting your entire LAN.

---

## Prerequisites

On the SBC (NanoPi Zero2, Raspberry Pi, etc.)

1. **Debian-based OS** (Raspberry Pi OS, Ubuntu, etc.)
2. **git**, **python3**, **python3-pip**
3. **Ansible**: `pip3 install --user ansible`

---

## Project Layout

```
FinGuard/
├── ansible.cfg
├── inventory/
│   ├── hosts
│   └── group_vars/
│       ├── all.yml
│       └── favicon.ico
├── playbook.yml
├── roles/
│   └── FinGuard/
│       ├── defaults/main.yml
│       ├── handlers/main.yml
│       ├── tasks/
│       │   ├── main.yml
│       │   ├── system.yml
│       │   ├── storage.yml
│       │   ├── security.yml
│       │   ├── networking.yml
│       │   ├── web.yml
│       │   ├── monitoring.yml
│       │   ├── maintenance.yml
│       │   └── services.yml
│       └── templates/
│           ├── nginx.conf.j2
│           ├── jellyfin-discovery-proxy.service.j2
│           ├── status-page.html.j2
│           └── [various systemd service templates]
└── README.md
```

---

## Configuration

Site-specific values live in `inventory/group_vars/all.yml`. Edit that file to configure your bridge:

```yaml
# inventory/group_vars/all.yml

# 1) Host identity
hostname: jellyfin

# 2) WireGuard (full wg0.conf text)
wg_conf: |
  [Interface]
  PrivateKey = <YOUR_PRIVATE_KEY>
  Address    = 10.192.1.X/32

  [Peer]
  PublicKey           = <SERVER_PUBLIC_KEY>
  Endpoint            = vpn.example.com:51820
  AllowedIPs          = 10.192.1.254/32
  PersistentKeepalive = 25

# 3) Services configuration - each gets its own subdomain
services:
  - { 
      name: "jellyfin",
      hostname: "jellyfin",  # Will be accessible at jellyfin.local
      upstream: "10.192.1.254:8096",
      path: "/",
      websocket: true,
      client_max_body_size: "1024m"
    }
  - { 
      name: "overseerr",
      hostname: "overseerr",  # Will be accessible at overseerr.local
      upstream: "10.192.1.254:5055",
      path: "/",
      websocket: false,
      client_max_body_size: "50m"
    }
  - { 
      name: "sonarr",
      hostname: "sonarr",  # Will be accessible at sonarr.local
      upstream: "10.192.1.254:8989",
      path: "/",
      websocket: true,
      client_max_body_size: "50m"
    }

# 4) Optional: set new password for 'pi'; leave blank to skip
pi_password: ""

# 5) For Discovery Proxy
jellyfin_server_url: "http://{{ services | selectattr('name', 'equalto', 'jellyfin') | map(attribute='upstream') | first }}"

# 6) FinGuard weekly‐update schedule
finguard_update_day: Wed # e.g. Mon, Tue, Wed, Thu, Fri, Sat, Sun
finguard_update_time: "03:00:00" # HH:MM:SS
```

Default values (timezone, WireGuard interface, binary paths, etc.) are defined in `roles/FinGuard/defaults/main.yml`.

---

## Deployment Steps

### Local execution

1. SSH into the SBC:
   ```bash
   ssh pi@<sbc-ip>
   ```
2. Prepare and fix the OS environment:
   ```bash
   sudo apt update
   sudo cp /etc/apt/sources.list /etc/apt/sources.list.backup
   sudo tee /etc/apt/sources.list > /dev/null << 'EOF'
   deb https://deb.debian.org/debian bookworm main non-free-firmware non-free contrib
   deb-src https://deb.debian.org/debian bookworm main non-free-firmware non-free contrib
   deb https://deb.debian.org/debian-security bookworm-security main
   deb-src https://deb.debian.org/debian-security bookworm-security main
   deb https://deb.debian.org/debian bookworm-backports main non-free-firmware non-free contrib
   deb-src https://deb.debian.org/debian bookworm-backports main non-free-firmware non-free contrib
   EOF
   sudo apt update
   sudo apt --fix-broken install -y
   sudo apt install -y python3 python3-pip git locales ansible
   sudo sed -i 's/^# *\(en_US.UTF-8 UTF-8\)/\1/' /etc/locale.gen
   sudo locale-gen
   sudo update-locale LANG=en_US.UTF-8
   ```
3. Clone the FinGuard repository:
   ```bash
   git clone https://github.com/jpkribs/FinGuard.git
   cd FinGuard
   ```
4. Edit site vars:
   ```bash
   nano inventory/group_vars/all.yml
   ```
5. Run the playbook:
   ```bash
   sudo ansible-playbook -c local playbook.yml
   ```

---

## Service Features

### Storage Optimization
- tmpfs mounts for logs and cache to reduce SD card wear
- Disabled swap and optimized journald settings
- Minimal logging configuration

### Security
- Self-signed SSL certificates for each service
- Automatic certificate renewal
- Secure NGINX configuration with modern TLS

### Monitoring
- Real-time status dashboard at `https://<hostname>.local/status`
- Network traffic monitoring with historical graphs
- System metrics (CPU, memory, disk, temperature)
- Service health monitoring

### Maintenance
- Weekly automated updates via systemd timers
- Automatic service restart on failure
- Update logs at `/var/log/FinGuard-update.log`

---

## Service Auto-Restart

All core services (WireGuard, NGINX, Jellyfin Discovery Proxy) are managed by systemd to:

- Enable at boot
- Restart on failure
- Auto-recovery from crashes or reboots

---

## Weekly Maintenance

A systemd timer runs weekly (configurable day/time) to:

1. Download the latest Jellyfin Discovery Proxy binary
2. Update the OS and installed packages
3. Restart critical services
4. Reboot if kernel updates require it

Timer logs are saved to `/var/log/FinGuard-update.log`.

---

## Verification

After deployment, each service will be accessible at its own subdomain:

- `https://jellyfin.local/` → Jellyfin Media Server
- `https://overseerr.local/` → Overseerr Request Management  
- `https://sonarr.local/` → Sonarr TV Management
- `https://<hostname>.local/status` → FinGuard Status Dashboard

The status dashboard provides:
- System metrics and health
- Network traffic monitoring
- Service status indicators
- Automatic updates on network interface changes

Jellyfin clients should also auto-discover your server via mDNS (UDP port 7359).

---

## Troubleshooting

- **Ansible errors**: Rerun with `-vvv` and verify SSH/`sudo` access.
- **Service not accessible**: Check service configuration in `all.yml` and verify upstream is reachable.
- **SSL certificate errors**: Certificates are self-signed; add security exceptions in browsers.
- **Discovery Proxy**: Check service status:
  ```bash
  systemctl status jellyfin-discovery-proxy
  ```
  Logs in `/var/log/syslog`.
- **mDNS issues**: Ensure Avahi is running:
  ```bash
  systemctl status avahi-daemon
  ```
  Confirm both devices are on the same subnet.
- **Status dashboard**: Check timer status:
  ```bash
  systemctl status status-update.timer
  ```

### Service Status Commands

Check individual service status:
```bash
systemctl status nginx
systemctl status wg-quick@wg0
systemctl status jellyfin-discovery-proxy
systemctl status mdns-publisher
```

View service logs:
```bash
journalctl -u nginx -f
journalctl -u jellyfin-discovery-proxy -f
```

---

## License

This project is licensed under the [MIT License](https://github.com/JPKribs/FinGuard/blob/main/LICENSE).
