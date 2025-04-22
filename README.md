# FinGuard

Minimal Debian SBC “boot & run” project to:

1. Install WireGuard (client).
2. Configure an NGINX reverse‑proxy for your defined services.
3. Advertise your Jellyfin server via mDNS using the Jellyfin Discovery Proxy.

---

## Goal and Architecture

FinGuard turns any small single-board computer (SBC) running Debian into a **dedicated WireGuard bridge** for your media ecosystem. Instead of installing a WireGuard client on every device or reconfiguring your router, you point all media clients at `http://<hostname>.local` and let FinGuard handle:

- **WireGuard**: Securely tunnel traffic from local devices into your remote network.
- **NGINX**: Proxy paths dynamically based on your defined services (default includes Jellyfin at `/` and optionally other services).
- **mDNS Discovery**: Advertise your Jellyfin server automatically to clients via the Discovery Proxy.

### Why a dedicated bridge?

- **Simplicity**: No per-device VPN configuration. Point all devices at a single `.local` address.
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
│   └── hosts
│   └── group_vars
│       └── all.yml
├── playbook.yml
├── roles/
│   └── FinGuard/
│       ├── defaults/main.yml
│       ├── tasks/main.yml
│       └── templates/
│           ├── nginx.conf.j2
│           └── jellyfin-discovery-proxy.service.j2
└── README.md
```

---

## Configuration

Site-specific values live in `inventory/group_vars/all.yml`. Edit that file to configure your bridge:

```yaml
# inventory/group_vars/all.yml

hostname: jellyfin
timezone: America/Denver

wg_conf: |
  [Interface]
  PrivateKey = <YOUR_PRIVATE_KEY>
  Address    = 10.192.1.X/32
  DNS        = 1.1.1.1

  [Peer]
  PublicKey           = <SERVER_PUBLIC_KEY>
  Endpoint            = vpn.example.com:51820
  AllowedIPs          = 10.192.1.254/32
  PersistentKeepalive = 60

services:
  - { ip: "10.192.1.254:8096", name: "jellyfin", path: "/" }
  - { ip: "10.192.1.254:5055", name: "other", path: "/other/" } # Ensure the path does not conflict with Jellyfin path. Use Jellyfin's Base URL to prevent conflicts.

pi_password: ""

jellyfin_server_url: "http://{{ services | selectattr('name', 'equalto', 'jellyfin') | map(attribute='ip') | first }}"
```

Defaults for other values (e.g., `wg_interface`, `discovery_repo`, paths, etc.) are defined in `roles/FinGuard/defaults/main.yml`.

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
   sudo sed -i 's|https://mirrors.aliyun.com/debian|https://deb.debian.org/debian|g' /etc/apt/sources.list
   sudo sed -i 's|bookworm main contrib non-free|bookworm main contrib non-free non-free-firmware|g' /etc/apt/sources.list
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

## Service Auto-Restart

All core services (WireGuard, NGINX, Jellyfin Discovery Proxy) are managed by systemd to:

- Enable at boot
- Restart on failure

This ensures the bridge recovers from crashes or reboots.

---

## Weekly Maintenance Cron

A cron job runs every Wednesday at 03:00 (timezone as specified) to:

1. Pull the latest Jellyfin Discovery Proxy code and rebuild.
2. Update the OS and installed packages:
   ```bash
   sudo apt update && sudo apt upgrade -y
   ```
3. Reboot the device to apply updates.

Cron logs are appended to `/var/log/FinGuard-update.log`.

---

## Verification

After deployment, access the following URLs:

- `http://<hostname>.local/` → Root Services
- `http://<hostname>.local/service-path` → Other Services

Jellyfin clients should also auto-discover your server via mDNS (UDP port 7359).

---

## Troubleshooting

- **Ansible errors**: Rerun with `-vvv` and verify SSH/`sudo` access.
- **NGINX 404**: Check your `_ip` variables; only non-empty ones generate locations.
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


---

## License

This project is licensed under the [MIT License](https://github.com/JPKribs/FinGuard/blob/main/LICENSE).
