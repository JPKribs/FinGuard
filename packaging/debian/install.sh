#!/bin/bash
set -e

# MARK: FinGuard Installation Script
echo "🛡️  FinGuard Installation Script"
echo "================================"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "❌ This script must be run as root (use sudo)"
    exit 1
fi

# Check for required tools
for cmd in systemctl dpkg; do
    if ! command -v $cmd &> /dev/null; then
        echo "❌ Required command '$cmd' not found"
        exit 1
    fi
done

# MARK: Find the .deb package
DEB_FILE=""
# Look for any finguard .deb file in common locations
for location in "./build/debian/finguard_"*"_amd64.deb" "./finguard_"*"_amd64.deb" "./"*"finguard"*".deb"; do
    if [ -f $location ]; then
        DEB_FILE="$location"
        break
    fi
done

if [ -z "$DEB_FILE" ]; then
    echo "❌ FinGuard .deb package not found!"
    echo "Please run 'make deb' or './packaging/debian/build-deb.sh' first"
    echo "Searched in:"
    echo "  - ./build/debian/"
    echo "  - ./ (current directory)"
    exit 1
fi

echo "📦 Found package: $DEB_FILE"

# Extract version from package name for display
PACKAGE_VERSION=$(echo "$DEB_FILE" | sed -n 's/.*finguard_\([^_]*\)_amd64\.deb/\1/p' || echo "unknown")
echo "🏷️  Version: $PACKAGE_VERSION"

# MARK: Install dependencies
echo "📥 Installing dependencies..."
apt update
apt install -y avahi-daemon systemd libcap2-bin

# MARK: Install the package
echo "🔧 Installing FinGuard..."
dpkg -i "$DEB_FILE" || {
    echo "📥 Resolving dependencies..."
    apt install -f -y
}

# MARK: Generate secure admin token if needed
CONFIG_FILE="/etc/finguard/config.yaml"
if [ -f "$CONFIG_FILE" ] && grep -q "REPLACE_ME_WITH_SECURE_TOKEN" "$CONFIG_FILE"; then
    echo "🔐 Generating secure admin token..."
    # Generate a 32-character random token
    NEW_TOKEN=$(openssl rand -hex 16 2>/dev/null || dd if=/dev/urandom bs=16 count=1 2>/dev/null | xxd -p | tr -d '\n')
    sed -i "s/REPLACE_ME_WITH_SECURE_TOKEN/$NEW_TOKEN/" "$CONFIG_FILE"
    echo "✅ Admin token updated in $CONFIG_FILE"
    echo "🔑 Your admin token: $NEW_TOKEN"
    echo ""
    echo "⚠️  IMPORTANT: Save this token! You'll need it to access the web interface."
    echo ""
fi

# MARK: Start the service
echo "🚀 Starting FinGuard service..."
systemctl daemon-reload
systemctl enable finguard
systemctl start finguard

# MARK: Check service status
sleep 2
if systemctl is-active --quiet finguard; then
    echo "✅ FinGuard is running successfully!"
    echo ""
    echo "🌐 Web interface: http://localhost:10000"
    echo "📊 Service status: sudo systemctl status finguard"
    echo "📋 View logs: journalctl -u finguard -f"
    echo ""
    echo "🔧 Configuration files:"
    echo "   Main config: /etc/finguard/config.yaml"
    echo "   Services:    /etc/finguard/services.yaml"
    echo "   WireGuard:   /etc/finguard/wireguard.yaml"
    echo "   Updates:     /etc/finguard/update.yaml"
    echo ""
    
    # Show admin token again
    if [ -f "$CONFIG_FILE" ]; then
        ADMIN_TOKEN=$(grep "admin_token:" "$CONFIG_FILE" | sed 's/.*admin_token: *"*\([^"]*\)"*.*/\1/')
        if [ "$ADMIN_TOKEN" != "REPLACE_ME_WITH_SECURE_TOKEN" ]; then
            echo "🔑 Your admin token: $ADMIN_TOKEN"
            echo ""
        fi
    fi
    
    echo "🎉 Installation completed successfully!"
else
    echo "❌ FinGuard failed to start"
    echo "📋 Check logs: journalctl -u finguard --no-pager"
    systemctl status finguard --no-pager || true
    exit 1
fi