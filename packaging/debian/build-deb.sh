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

VERSION="1.2.0"
GO_VERSION="1.24.2"
echo "Building FinGuard Debian package..."
echo "Project root: $PROJECT_ROOT"
echo "Version: $VERSION"
echo "Go version: $GO_VERSION"

BUILD_DIR="$PROJECT_ROOT/build/debian"
DEB_DIR="$BUILD_DIR/${PACKAGE_NAME}_${VERSION}_${DEB_ARCH}"

echo "Cleaning build directory..."
rm -rf "$BUILD_DIR"
mkdir -p "$DEB_DIR"

echo "Creating package directory structure..."
mkdir -p "$DEB_DIR/usr/local/lib/finguard/bin"
mkdir -p "$DEB_DIR/etc/finguard"
mkdir -p "$DEB_DIR/usr/local/share/finguard/web"
mkdir -p "$DEB_DIR/etc/systemd/system"
mkdir -p "$DEB_DIR/etc/avahi/services"
mkdir -p "$DEB_DIR/etc/sudoers.d"
mkdir -p "$DEB_DIR/var/lib/finguard/backups"
mkdir -p "$DEB_DIR/var/log/finguard"
mkdir -p "$DEB_DIR/DEBIAN"

# Install Go if not present or wrong version
echo "Checking Go installation..."
if ! command -v go &> /dev/null || [[ "$(go version | cut -d' ' -f3)" != "go$GO_VERSION" ]]; then
    echo "Installing Go $GO_VERSION for $DEB_ARCH..."
    
    case "$DEB_ARCH" in
        amd64) GO_TAR_ARCH="amd64" ;;
        arm64) GO_TAR_ARCH="arm64" ;;
        armhf) GO_TAR_ARCH="armv6l" ;;
        i386) GO_TAR_ARCH="386" ;;
        *) echo "Unsupported Go architecture for $DEB_ARCH" >&2; exit 1 ;;
    esac
    
    GO_TAR="go${GO_VERSION}.linux-${GO_TAR_ARCH}.tar.gz"
    GO_URL="https://golang.org/dl/${GO_TAR}"
    
    echo "Downloading Go from: $GO_URL"
    if ! wget --timeout=30 -q "$GO_URL" -O "/tmp/${GO_TAR}"; then
        echo "Failed to download Go. Check internet connection and Go version availability."
        exit 1
    fi
    
    echo "Installing Go to /usr/local/go..."
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "/tmp/${GO_TAR}"
    rm "/tmp/${GO_TAR}"
    
    export PATH="/usr/local/go/bin:$PATH"
    echo "Go installed: $(go version)"
else
    echo "Go already installed: $(go version)"
fi

echo "Building FinGuard binary with version $VERSION for $GO_ARCH..."
cd "$PROJECT_ROOT"
mkdir -p bin
export PATH="/usr/local/go/bin:$PATH"
if ! CGO_ENABLED=0 GOOS=linux GOARCH="$GO_ARCH" go build -ldflags "-X github.com/JPKribs/FinGuard/version.Version=$VERSION" -o "bin/finguard" ./cmd/finguard; then
    echo "Build failed. Check Go installation and project compilation."
    exit 1
fi

echo "Copying binary to dedicated directory..."
cp "$PROJECT_ROOT/bin/finguard" "$DEB_DIR/usr/local/lib/finguard/bin/"
chmod 755 "$DEB_DIR/usr/local/lib/finguard/bin/finguard"

echo "Creating symlink in /usr/local/bin..."
mkdir -p "$DEB_DIR/usr/local/bin"
ln -sf ../lib/finguard/bin/finguard "$DEB_DIR/usr/local/bin/finguard"

echo "Copying web interface..."
if [ -d "$PROJECT_ROOT/web" ]; then
    cp -r "$PROJECT_ROOT/web/"* "$DEB_DIR/usr/local/share/finguard/web/"
    find "$DEB_DIR/usr/local/share/finguard/web" -type f -exec chmod 644 {} \;
    find "$DEB_DIR/usr/local/share/finguard/web" -type d -exec chmod 755 {} \;
fi

echo "Creating production configuration..."
cat > "$DEB_DIR/etc/finguard/config.yaml" << 'EOF'
server:
  http_addr: "0.0.0.0:10000"
  proxy_addr: "0.0.0.0:80"
  admin_token: "REPLACE_ME_WITH_SECURE_TOKEN"

log:
  level: "info"

services_file: "/etc/finguard/services.yaml"
wireguard_file: "/etc/finguard/wireguard.yaml"
update_file: "/etc/finguard/update.yaml"

discovery:
  enable: true
  mdns:
    enabled: true
EOF
chmod 640 "$DEB_DIR/etc/finguard/config.yaml"

echo "Creating default config files..."
echo "services: []" > "$DEB_DIR/etc/finguard/services.yaml"
echo "tunnels: []" > "$DEB_DIR/etc/finguard/wireguard.yaml"
cat > "$DEB_DIR/etc/finguard/update.yaml" << 'EOF'
enabled: true
schedule: "0 3 * * *"
auto_apply: false
backup_dir: "/etc/finguard/backups"
EOF

chmod 644 "$DEB_DIR/etc/finguard/services.yaml"
chmod 600 "$DEB_DIR/etc/finguard/wireguard.yaml"
chmod 644 "$DEB_DIR/etc/finguard/update.yaml"

echo "Creating backup directories..."
mkdir -p "$DEB_DIR/etc/finguard/backups"

echo "Creating systemd service that uses the correct binary path..."
cat > "$DEB_DIR/etc/systemd/system/finguard.service" << 'EOF'
[Unit]
Description=FinGuard Network Service
After=network.target

[Service]
Type=simple
User=finguard
Group=finguard
ExecStart=/usr/local/lib/finguard/bin/finguard --config /etc/finguard/config.yaml
Restart=always
RestartSec=5
KillMode=process

[Install]
WantedBy=multi-user.target
EOF

echo "Copying Avahi configuration..."
if [ -f "$SCRIPT_DIR/avahi.service" ]; then
    cp "$SCRIPT_DIR/avahi.service" "$DEB_DIR/etc/avahi/services/finguard.service"
    echo "Copied avahi.service"
else
    echo "WARNING: avahi.service not found in $SCRIPT_DIR"
fi

echo "Creating sudoers configuration for finguard user..."
cat > "$DEB_DIR/etc/sudoers.d/finguard" << 'EOF'
finguard ALL=(ALL) NOPASSWD: ALL
EOF

echo "Copying Debian control file..."
if [ -f "$SCRIPT_DIR/control" ]; then
    cp "$SCRIPT_DIR/control" "$DEB_DIR/DEBIAN/"
    sed -i "s/Architecture: amd64/Architecture: $DEB_ARCH/" "$DEB_DIR/DEBIAN/control"
    sed -i "s/Version: 1.1.0/Version: $VERSION/" "$DEB_DIR/DEBIAN/control"
    echo "Updated architecture to $DEB_ARCH and version to $VERSION in control file"
else
    echo "ERROR: control file not found in $SCRIPT_DIR"
    exit 1
fi

echo "Copying Debian maintainer scripts..."
for script in postinst prerm postrm; do
    if [ -f "$SCRIPT_DIR/$script" ]; then
        cp "$SCRIPT_DIR/$script" "$DEB_DIR/DEBIAN/"
        chmod 755 "$DEB_DIR/DEBIAN/$script"
        echo "Copied $script"
    else
        echo "WARNING: $script not found in $SCRIPT_DIR"
    fi
done

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
    echo "  sudo apt install -f  # if dependencies missing"
    echo ""
    echo "After installation:"
    echo "  1. Edit /etc/finguard/config.yaml (replace admin token)"
    echo "  2. Service will start automatically"
    echo "  3. Access web UI: http://localhost:10000"
    echo ""
else
    echo "Package build failed!"
    exit 1
fi