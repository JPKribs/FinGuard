#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
PACKAGE_NAME="finguard"

case "$(uname -m)" in
    x86_64) GO_ARCH="amd64"; DEB_ARCH="amd64" ;;
    aarch64|arm64) GO_ARCH="arm64"; DEB_ARCH="arm64" ;;
    armv7l) GO_ARCH="arm"; DEB_ARCH="armhf" ;;
    i386|i686) GO_ARCH="386"; DEB_ARCH="i386" ;;
    *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

echo "Detected architecture: $(uname -m) -> Go: $GO_ARCH, Debian: $DEB_ARCH"

VERSION="1.1.0"
echo "Building FinGuard Debian package..."
echo "Project root: $PROJECT_ROOT"
echo "Version: $VERSION"

BUILD_DIR="$PROJECT_ROOT/build/debian"
DEB_DIR="$BUILD_DIR/${PACKAGE_NAME}_${VERSION}_${DEB_ARCH}"

echo "Cleaning build directory..."
rm -rf "$BUILD_DIR"
mkdir -p "$DEB_DIR"

echo "Creating package directory structure..."
mkdir -p "$DEB_DIR/usr/local/bin"
mkdir -p "$DEB_DIR/etc/finguard"
mkdir -p "$DEB_DIR/usr/local/share/finguard/web"
mkdir -p "$DEB_DIR/etc/systemd/system"
mkdir -p "$DEB_DIR/var/lib/finguard"
mkdir -p "$DEB_DIR/var/log/finguard"
mkdir -p "$DEB_DIR/DEBIAN"

echo "Building FinGuard binary with version $VERSION for $GO_ARCH..."
cd "$PROJECT_ROOT"
mkdir -p bin
if ! CGO_ENABLED=0 GOOS=linux GOARCH="$GO_ARCH" go build -o "bin/finguard" ./cmd/finguard; then
    echo "Build failed. Make sure you have Go installed and the project compiles."
    exit 1
fi

echo "Copying binary..."
cp "$PROJECT_ROOT/bin/finguard" "$DEB_DIR/usr/local/bin/"
chmod 755 "$DEB_DIR/usr/local/bin/finguard"

echo "Copying web interface..."
if [ -d "$PROJECT_ROOT/web" ]; then
    cp -r "$PROJECT_ROOT/web/"* "$DEB_DIR/usr/local/share/finguard/web/"
    find "$DEB_DIR/usr/local/share/finguard/web" -type f -exec chmod 644 {} \;
    find "$DEB_DIR/usr/local/share/finguard/web" -type d -exec chmod 755 {} \;
fi

echo "Copying configuration files..."
if [ -f "$PROJECT_ROOT/packaging/config-production.yaml" ]; then
    cp "$PROJECT_ROOT/packaging/config-production.yaml" "$DEB_DIR/etc/finguard/config.yaml"
    chmod 640 "$DEB_DIR/etc/finguard/config.yaml"
fi

echo "services: []" > "$DEB_DIR/etc/finguard/services.yaml"
echo "tunnels: []" > "$DEB_DIR/etc/finguard/wireguard.yaml"
cat > "$DEB_DIR/etc/finguard/update.yaml" << 'EOF'
enabled: false
schedule: "0 3 * * *"
auto_apply: false
backup_dir: "./backups"
EOF

chmod 644 "$DEB_DIR/etc/finguard/services.yaml"
chmod 600 "$DEB_DIR/etc/finguard/wireguard.yaml"
chmod 644 "$DEB_DIR/etc/finguard/update.yaml"

echo "Copying systemd service..."
cat > "$DEB_DIR/etc/systemd/system/finguard.service" << 'EOF'
[Unit]
Description=FinGuard - WireGuard Proxy with Web Management
Documentation=https://github.com/JPKribs/FinGuard
After=network-online.target avahi-daemon.service
Wants=network-online.target
RequiresMountsFor=/var/lib/finguard

[Service]
Type=simple
User=finguard
Group=finguard
ExecStart=/usr/local/bin/finguard --config /etc/finguard/config.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=10
WorkingDirectory=/var/lib/finguard

# Network capabilities for TUN device creation and port 80 binding
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=finguard

[Install]
WantedBy=multi-user.target
EOF

echo "Creating Debian control file..."
cat > "$DEB_DIR/DEBIAN/control" << EOF
Package: finguard
Version: $VERSION
Section: net
Priority: optional
Architecture: $DEB_ARCH
Depends: libc6, avahi-daemon, systemd, libcap2-bin
Maintainer: Joseph Parker Kribs <joseph@kribs.net>
Description: High-performance userspace WireGuard proxy with web management
 FinGuard is a modern WireGuard proxy solution that provides:
  - Userspace WireGuard implementation
  - HTTP reverse proxy with subdomain routing
  - Web-based management interface
  - mDNS service discovery via Avahi
  - Real-time connection monitoring
  - Automatic route management
  - Auto-update system with GitHub integration
 .
 This package includes systemd integration for production deployment.
EOF

echo "Copying Debian control files..."
for file in postinst prerm postrm; do
    if [ -f "$PROJECT_ROOT/packaging/debian/$file" ]; then
        cp "$PROJECT_ROOT/packaging/debian/$file" "$DEB_DIR/DEBIAN/"
        chmod 755 "$DEB_DIR/DEBIAN/$file"
    fi
done
chmod 644 "$DEB_DIR/DEBIAN/control"

cat > "$DEB_DIR/DEBIAN/conffiles" << EOF
/etc/finguard/config.yaml
/etc/finguard/services.yaml
/etc/finguard/wireguard.yaml
/etc/finguard/update.yaml
EOF

INSTALLED_SIZE=$(du -sk "$DEB_DIR" | cut -f1)
echo "Installed-Size: $INSTALLED_SIZE" >> "$DEB_DIR/DEBIAN/control"

echo "Building .deb package..."
cd "$BUILD_DIR"
dpkg-deb --build "${PACKAGE_NAME}_${VERSION}_${DEB_ARCH}"

DEB_FILE="${BUILD_DIR}/${PACKAGE_NAME}_${VERSION}_${DEB_ARCH}.deb"
if [ -f "$DEB_FILE" ]; then
    echo ""
    echo "Package built successfully!"
    echo "Package: $DEB_FILE"
    echo "Size: $(du -h "$DEB_FILE" | cut -f1)"
    echo "Version: $VERSION"
    echo "Architecture: $DEB_ARCH"
    echo ""
    echo "To install:"
    echo "  sudo dpkg -i $DEB_FILE"
    echo ""
else
    echo "Package build failed!"
    exit 1
fi