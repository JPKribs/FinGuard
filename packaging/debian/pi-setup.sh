#!/bin/bash

set -e

echo "Starting Debian NanoPi-Zero2 setup..."

# MARK: backup_sources_list
backup_sources_list() {
    if [ -f /etc/apt/sources.list ]; then
        cp /etc/apt/sources.list /etc/apt/sources.list.backup.$(date +%Y%m%d_%H%M%S)
        echo "Backed up existing sources.list"
    fi
}

# MARK: setup_debian_repos
setup_debian_repos() {
    DEBIAN_VERSION=$(lsb_release -cs 2>/dev/null || echo "bookworm")
    
    cat > /etc/apt/sources.list << EOF
deb http://deb.debian.org/debian ${DEBIAN_VERSION} main contrib non-free non-free-firmware
deb-src http://deb.debian.org/debian ${DEBIAN_VERSION} main contrib non-free non-free-firmware

deb http://deb.debian.org/debian ${DEBIAN_VERSION}-updates main contrib non-free non-free-firmware
deb-src http://deb.debian.org/debian ${DEBIAN_VERSION}-updates main contrib non-free non-free-firmware

deb http://security.debian.org/debian-security ${DEBIAN_VERSION}-security main contrib non-free non-free-firmware
deb-src http://security.debian.org/debian-security ${DEBIAN_VERSION}-security main contrib non-free non-free-firmware
EOF
    
    echo "Updated sources.list with official Debian repositories"
}

# MARK: clean_additional_repos
clean_additional_repos() {
    if [ -d /etc/apt/sources.list.d ]; then
        find /etc/apt/sources.list.d -name "*.list" -exec mv {} {}.disabled \;
        echo "Disabled additional repository files"
    fi
}

# MARK: fix_repositories
fix_repositories() {
    apt-get clean
    apt-get update
    apt-get install -f -y
    dpkg --configure -a
    apt-get autoremove -y
    apt-get autoclean
    echo "Fixed and cleaned repositories"
}

# MARK: disable_swap
disable_swap() {
    swapoff -a
    sed -i '/swap/d' /etc/fstab
    systemctl mask swap.target
    echo "Disabled all swap to protect eMMC"
}

# MARK: optimize_logging
optimize_logging() {
    mkdir -p /etc/systemd/journald.conf.d
    cat > /etc/systemd/journald.conf.d/99-emmc-friendly.conf << 'EOF'
[Journal]
SystemMaxUse=50M
SystemMaxFileSize=10M
RuntimeMaxUse=20M
RuntimeMaxFileSize=5M
MaxFileSec=1day
MaxRetentionSec=3day
SyncIntervalSec=300
Storage=volatile
Compress=yes
EOF
    
    systemctl restart systemd-journald
    echo "Configured minimal journald logging"
}

# MARK: configure_tmpfs
configure_tmpfs() {
    cat >> /etc/fstab << 'EOF'

# tmpfs mounts to reduce eMMC writes
tmpfs /tmp tmpfs defaults,noatime,nosuid,nodev,noexec,mode=1777,size=100M 0 0
tmpfs /var/tmp tmpfs defaults,noatime,nosuid,nodev,noexec,mode=1777,size=50M 0 0
tmpfs /var/log tmpfs defaults,noatime,nosuid,nodev,noexec,mode=0755,size=50M 0 0
tmpfs /var/cache/apt tmpfs defaults,noatime,nosuid,nodev,noexec,mode=0755,size=100M 0 0
EOF
    echo "Added tmpfs mounts for high-write directories"
}

# MARK: optimize_filesystem
optimize_filesystem() {
    cp /etc/fstab /etc/fstab.backup.$(date +%Y%m%d_%H%M%S)
    
    sed -i 's/defaults/defaults,noatime,commit=60/' /etc/fstab
    
    if ! grep -q "vm.dirty_ratio" /etc/sysctl.conf; then
        cat >> /etc/sysctl.conf << 'EOF'

# eMMC optimization settings
vm.dirty_ratio = 5
vm.dirty_background_ratio = 2
vm.dirty_writeback_centisecs = 1500
vm.dirty_expire_centisecs = 1500
vm.swappiness = 1
vm.vfs_cache_pressure = 50
EOF
    fi
    
    echo "Optimized filesystem settings for eMMC longevity"
}

# MARK: disable_unnecessary_services
disable_unnecessary_services() {
    SERVICES_TO_DISABLE=(
        "man-db.timer"
        "apt-daily.timer"
        "apt-daily-upgrade.timer"
        "logrotate.timer"
        "fstrim.timer"
        "e2scrub_all.timer"
        "systemd-tmpfiles-clean.timer"
    )
    
    for service in "${SERVICES_TO_DISABLE[@]}"; do
        if systemctl is-enabled "$service" &>/dev/null; then
            systemctl disable "$service"
            echo "Disabled $service"
        fi
    done
}

# MARK: setup_log_rotation
setup_log_rotation() {
    cat > /etc/logrotate.d/emmc-friendly << 'EOF'
/var/log/*.log {
    size 1M
    rotate 2
    compress
    delaycompress
    missingok
    notifempty
    create 644 root root
    postrotate
        /bin/systemctl reload-or-restart rsyslog > /dev/null 2>&1 || true
    endscript
}
EOF
    echo "Configured aggressive log rotation"
}

# MARK: set_locale
set_locale() {
    apt-get install -y locales
    
    sed -i 's/^# *en_US.UTF-8/en_US.UTF-8/' /etc/locale.gen
    locale-gen
    
    update-locale LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8
    
    echo "Set locale to en_US.UTF-8"
}

# MARK: set_timezone
set_timezone() {
    echo "Setting timezone to Mountain Time..."
    
    # Try timedatectl first
    if timedatectl set-timezone America/Denver 2>/dev/null; then
        echo "Set timezone to Mountain Time (America/Denver)"
    else
        echo "timedatectl failed, trying alternative method..."
        
        # Fallback method using ln
        if [ -f /usr/share/zoneinfo/America/Denver ]; then
            ln -sf /usr/share/zoneinfo/America/Denver /etc/localtime
            echo "America/Denver" > /etc/timezone
            echo "Set timezone to Mountain Time (fallback method)"
        else
            echo "Warning: Could not set timezone - will use system default"
        fi
    fi
    
    # Disable automatic time sync to avoid timeout issues during setup
    timedatectl set-ntp false 2>/dev/null || true
}

# MARK: set_hostname
set_hostname() {
    hostnamectl set-hostname FinGuard
    
    sed -i 's/^127\.0\.1\.1.*/127.0.1.1\tFinGuard/' /etc/hosts
    
    if ! grep -q "127.0.1.1" /etc/hosts; then
        echo "127.0.1.1	FinGuard" >> /etc/hosts
    fi
    
    echo "Set hostname to FinGuard"
}

# MARK: create_update_script
create_update_script() {
    cat > /usr/local/bin/system-update.sh << 'EOF'
#!/bin/bash

LOG_FILE="/var/log/system-update.log"
REBOOT_REQUIRED="/var/run/reboot-required"

log_message() {
    echo "$(date '+%Y-%m-%d %H:%M:%S'): $1" | tee -a "$LOG_FILE"
}

log_message "Starting system update"

apt-get update >> "$LOG_FILE" 2>&1

UPGRADABLE=$(apt list --upgradable 2>/dev/null | grep -c upgradable || echo 0)

if [ "$UPGRADABLE" -gt 0 ]; then
    log_message "Found $UPGRADABLE packages to upgrade"
    
    DEBIAN_FRONTEND=noninteractive apt-get upgrade -y >> "$LOG_FILE" 2>&1
    
    log_message "Package upgrade completed"
    
    if [ -f "$REBOOT_REQUIRED" ] || [ -n "$(find /var/run -name 'reboot-required*' 2>/dev/null)" ]; then
        log_message "Reboot required, initiating reboot in 30 seconds"
        sleep 30
        reboot
    else
        log_message "No reboot required"
    fi
else
    log_message "No packages to upgrade"
fi

log_message "System update completed"
EOF
    
    chmod +x /usr/local/bin/system-update.sh
    echo "Created update script at /usr/local/bin/system-update.sh"
}

# MARK: setup_cron_job
setup_cron_job() {
    CRON_JOB="0 3 * * 3 /usr/local/bin/system-update.sh"
    
    if ! crontab -l 2>/dev/null | grep -F "$CRON_JOB" > /dev/null; then
        (crontab -l 2>/dev/null; echo "$CRON_JOB") | crontab -
        echo "Added cron job to run updates every Wednesday at 3 AM"
    else
        echo "Cron job already exists"
    fi
    
    systemctl enable cron
    systemctl start cron
}

# MARK: main_execution
main() {
    if [ "$EUID" -ne 0 ]; then
        echo "This script must be run as root"
        exit 1
    fi
    
    local hostname="${1:-FinGuard}"
    
    if [ "$#" -eq 0 ]; then
        echo "No hostname provided, using default: FinGuard"
        echo "Usage: $0 [hostname] - next time you can specify a custom hostname"
        echo ""
    fi
    
    echo "=== Backing up current configuration ==="
    backup_sources_list
    
    echo "=== Setting up Debian repositories ==="
    setup_debian_repos
    clean_additional_repos
    
    echo "=== Fixing repositories ==="
    fix_repositories
    
    echo "=== Optimizing for eMMC longevity ==="
    disable_swap
    optimize_logging
    configure_tmpfs
    optimize_filesystem
    disable_unnecessary_services
    setup_log_rotation
    
    echo "=== Setting locale ==="
    set_locale
    
    echo "=== Setting timezone ==="
    set_timezone
    
    echo "=== Setting hostname ==="
    set_hostname "$hostname"
    
    echo "=== Installing WireGuard ==="
    install_wireguard
    
    echo "=== Creating update script ==="
    create_update_script
    
    echo "=== Setting up automated updates ==="
    setup_cron_job
    
    echo ""
    echo "Setup completed successfully!"
    echo "- Official Debian repositories configured"
    echo "- eMMC longevity optimizations applied (swap disabled, minimal logging, tmpfs)"
    echo "- Locale set to en_US.UTF-8"
    echo "- Timezone set to Mountain Time"
    echo "- Hostname set to $hostname"
    echo "- WireGuard installed and ready"
    echo "- Update script created at /usr/local/bin/system-update.sh"
    echo "- Automated updates scheduled for Wednesdays at 3 AM"
    echo ""
    echo "IMPORTANT: A reboot is required to activate all eMMC optimizations."
    echo "Run: sudo reboot"
}

main "$@"