FinGuard

Minimal Debian SBC “boot & run” project to:
	1.	Install WireGuard (client).
	2.	Configure an NGINX reverse-proxy for Jellyfin, Overseerr/Jellyseerr, JFA-GO, Jellyfin Vue.
	3.	Advertise your Jellyfin server via mDNS using the Jellyfin Discovery Proxy.

⸻

Goal and Architecture

FinGuard turns any small single-board computer (SBC) running Debian into a dedicated WireGuard bridge for your media ecosystem. Instead of installing a WireGuard client on every device or reconfiguring your router, you point all media clients at http://<hostname>.local and let FinGuard handle:
	•	WireGuard: Securely tunnel traffic from local devices into your remote network.
	•	NGINX: Proxy paths to Jellyfin, Overseerr/Jellyseerr, JFA-GO, or Jellyfin Vue based on URL.
	•	mDNS Discovery: Advertise your Jellyfin server automatically to clients via the Discovery Proxy.

Why a dedicated bridge?
	•	Simplicity: No per-device VPN configuration. Point all devices at a single .local address.
	•	Compatibility: Older devices or smart TVs often lack WireGuard support. The bridge handles encryption.
	•	Isolation: You can firewall or monitor the bridge separately, without affecting your entire LAN.

⸻

Prerequisites

On the SBC (NanoPi Zero2, Raspberry Pi, etc.)
	•	Debian-based OS (Raspberry Pi OS, Ubuntu, etc.)
	•	git, python3, python3-pip
	•	Ansible (pip3 install --user ansible)

⸻

Project Layout

fin_guard/
├── ansible.cfg           # Ansible settings
├── inventory/
│   └── hosts             # Hosts (local or remote)
├── group_vars/
│   └── all.yml           # Global variables (wg_conf, IPs, hostname, timezone)
├── playbook.yml          # Entry playbook
├── roles/
│   └── finguard/         # Main role
│       ├── defaults/main.yml
│       ├── tasks/main.yml
│       └── templates/
│           ├── nginx.conf.j2
│           └── jellyfin-discovery-proxy.service.j2
└── README.md             # You are here!



⸻

Configuration (group_vars/all.yml)

Edit only these values to customize your bridge:

# Host identity (no .local)
hostname: jellyfin

# System timezone (used for cron schedules)
timezone: America/Denver

# Full WireGuard client config (wg0.conf)
wg_conf: |
  [Interface]
  PrivateKey = <YOUR_PRIVATE_KEY>
  Address    = 10.0.0.50/24
  DNS        = 1.1.1.1

  [Peer]
  PublicKey           = <SERVER_PUBLIC_KEY>
  Endpoint            = vpn.example.com:51820
  AllowedIPs          = 10.0.0.0/24
  PersistentKeepalive = 25

# Upstream service endpoints (IP:port). Leave blank to skip
jellyfin_ip: 10.0.0.123:8096
overseerr_ip: 10.0.0.124:5055
jfa_go_ip: 10.0.0.125:6600
jellyfin_vue_ip: 10.0.0.126:8080

# Optionally reset the 'pi' user password; leave empty to skip
pi_password: ""

# URL for Discovery Proxy to point at
jellyfin_server_url: "http://10.0.0.123:8096"

	•	timezone: Defaults to America/Denver. Controls the system timezone and cron scheduling.
	•	wg_conf: Paste your complete wg0.conf; the role writes it to /etc/wireguard/wg0.conf.
	•	Service IPs: Only non-empty entries generate NGINX locations.
	•	pi_password: If set, updates the pi user password.

⸻

Deployment Steps

A) Local execution (on the SBC)
	1.	SSH into SBC:

ssh pi@<sbc-ip>


	2.	Install prerequisites:

sudo apt update && sudo apt install -y python3 python3-pip git
pip3 install --user ansible
export PATH=$HOME/.local/bin:$PATH


	3.	Clone FinGuard:

git clone https://github.com/youruser/fin_guard.git
cd fin_guard


	4.	Edit variables: nano group_vars/all.yml
	5.	Run playbook:

ansible-playbook -c local playbook.yml



B) Remote execution (from your Mac)
	1.	Install Ansible: brew install ansible
	2.	Edit inventory (inventory/hosts):

[fin_guard]
192.168.1.42 ansible_user=pi ansible_ssh_private_key_file=~/.ssh/id_rsa


	3.	Clone & configure on your Mac:

git clone https://github.com/youruser/fin_guard.git
cd fin_guard
nano group_vars/all.yml


	4.	Run playbook:

ansible-playbook playbook.yml



⸻

Service Auto-Restart

All core services (WireGuard, NGINX, Jellyfin Discovery Proxy) are configured under systemd to:
	•	Enable at boot
	•	Restart on failure

This ensures the bridge recovers from crashes or reboots.

⸻

Weekly Maintenance Cron

A cron job runs every Wednesday at 03:00 in the specified timezone to:
	1.	Pull the latest Jellyfin Discovery Proxy code and rebuild.
	2.	Update the OS and installed packages (sudo apt update && sudo apt upgrade -y).
	3.	Reboot the device to apply updates.

Cron logs are appended to /var/log/fin_guard_update.log.

⸻

Verification

After deployment, access:

http://<hostname>.local/          → Jellyfin
http://<hostname>.local/request   → Overseerr/Jellyseerr
http://<hostname>.local/account   → JFA-GO
http://<hostname>.local/vue       → Jellyfin Vue

Plus, Jellyfin clients should auto-discover your server via mDNS (UDP 7359).

⸻

Troubleshooting
	•	Ansible errors: Rerun with -vvv and verify SSH/sudo.
	•	NGINX 404: Check your *_ip variables; only non-empty ones are used.
	•	Discovery Proxy: systemctl status jellyfin-discovery-proxy & logs in /var/log/syslog.
	•	mDNS issues: Ensure Avahi is running (systemctl status avahi-daemon) and both devices share a subnet.

⸻

License

This project is licensed under the MIT License.