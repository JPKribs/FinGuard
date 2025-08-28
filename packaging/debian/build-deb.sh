#!/bin/bash
set -e

# MARK: FinGuard Debian Package Builder
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PACKAGE_NAME="finguard"
ARCH="amd64"

# MARK: Get version from main.go or git tag
get_version() {
    local version=""
    
    # Try to get version from main.go
    if [ -f "$PROJECT_ROOT/main.go" ]; then
        version=$(grep -E 'Version\s*=\s*"[^"]*"' "$PROJECT_ROOT/main.go" | sed 's/.*Version\s*=\s*"\([^"]*\)".*/\1/')
    fi
    
    # If not found in main.go, try git tag
    if [ -z "$version" ] && command -v git >/dev/null 2>&1; then
        if git -C "$PROJECT_ROOT" describe --tags --exact-match 2>/dev/null; then
            version=$(git -C "$PROJECT_ROOT" describe --tags --exact-match 2>/dev/null | sed 's/^v//')
        fi
    fi
    
    # If still not found, try git describe
    if [ -z "$version" ] && command -v git >/dev/null 2>&1; then
        if git -C "$PROJECT_ROOT" describe --tags 2>/dev/null; then
            version=$(git -C "$PROJECT_ROOT" describe --tags 2>/dev/null | sed 's/^v//' | sed 's/-g[a-f0-9]*$//')
        fi
    fi
    
    # Default fallback
    if [ -z "$version" ]; then
        version="1.0.0-dev"
        echo "Warning: Could not determine version, using default: $version" >&2
    fi
    
    echo "$version"
}

# Get version
VERSION=$(get_version)
echo "Building FinGuard Debian package..."
echo "Project root: $PROJECT_ROOT"
echo "Version: $VERSION"

# MARK: Clean and create build directories
BUILD_DIR="$PROJECT_ROOT/build/debian"
DEB_DIR="$BUILD_DIR/${PACKAGE_NAME}_${VERSION}_${ARCH}"

echo "Cleaning build directory..."
rm -rf "$BUILD_DIR"
mkdir -p "$DEB_DIR"

# MARK: Create directory structure
echo "Creating package directory structure..."
mkdir -p "$DEB_DIR/usr/local/bin"
mkdir -p "$DEB_DIR/etc/finguard"
mkdir -p "$DEB_DIR/usr/local/share/finguard/web"
mkdir -p "$DEB_DIR/etc/systemd/system"
mkdir -p "$DEB_DIR/var/lib/finguard"
mkdir -p "$DEB_DIR/var/log/finguard"
mkdir -p "$DEB_DIR/DEBIAN"

# MARK: Build the binary with version info
echo "Building FinGuard binary with version $VERSION..."
cd "$PROJECT_ROOT"
if ! CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=$VERSION" -o "bin/finguard"; then
    echo "Build failed. Make sure you have Go installed and the project compiles."
    exit 1
fi

# MARK: Copy files to package structure
echo "Copying binary..."
cp "$PROJECT_ROOT/bin/finguard" "$DEB_DIR/usr/local/bin/"
chmod 755 "$DEB_DIR/usr/local/bin/finguard"

echo "Copying web interface..."
cp -r "$PROJECT_ROOT/web/"* "$DEB_DIR/usr/local/share/finguard/web/"
find "$DEB_DIR/usr/local/share/finguard/web" -type f -exec chmod 644 {} \;
find "$DEB_DIR/usr/local/share/finguard/web" -type d -exec chmod 755 {} \;

echo "Copying configuration files..."
if [ -f "$PROJECT_ROOT/config-production.yaml" ]; then
    cp "$PROJECT_ROOT/config-production.yaml" "$DEB_DIR/etc/finguard/config.yaml"
    chmod 640 "$DEB_DIR/etc/finguard/config.yaml"
elif [ -f "$PROJECT_ROOT/config.yaml" ]; then
    cp "$PROJECT_ROOT/config.yaml" "$DEB_DIR/etc/finguard/"
    chmod 640 "$DEB_DIR/etc/finguard/config.yaml"
fi

# Create empty service and wireguard files if they don't exist
if [ ! -f "$DEB_DIR/etc/finguard/services.yaml" ]; then
    echo "services: []" > "$DEB_DIR/etc/finguard/services.yaml"
fi

if [ ! -f "$DEB_DIR/etc/finguard/wireguard.yaml" ]; then
    echo "tunnels: []" > "$DEB_DIR/etc/finguard/wireguard.yaml"
fi

if [ ! -f "$DEB_DIR/etc/finguard/update.yaml" ]; then
    cat > "$DEB_DIR/etc/finguard/update.yaml" << 'EOF'
enabled: false
schedule: "0 3 * * *"
auto_apply: false
backup_dir: "./backups"
EOF
fi

chmod 644 "$DEB_DIR/etc/finguard/services.yaml"
chmod 600 "$DEB_DIR/etc/finguard/wireguard.yaml"
chmod 644 "$DEB_DIR/etc/finguard/update.yaml"

echo "Copying systemd service..."
cp "$PROJECT_ROOT/packaging/debian/finguard.service" "$DEB_DIR/etc/systemd/system/"
chmod 644 "$DEB_DIR/etc/systemd/system/finguard.service"

# MARK: Create control file with dynamic version
echo "Creating Debian control file..."
cat > "$DEB_DIR/DEBIAN/control" << EOF
Package: finguard
Version: $VERSION
Section: net
Priority: optional
Architecture: amd64
Depends: libc6, avahi-daemon, systemd
Maintainer: Joseph Parker Kribs <your-email@example.com>
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

# MARK: Copy other Debian control files
echo "Copying Debian control files..."
cp "$PROJECT_ROOT/packaging/debian/postinst" "$DEB_DIR/DEBIAN/"
cp "$PROJECT_ROOT/packaging/debian/prerm" "$DEB_DIR/DEBIAN/"
cp "$PROJECT_ROOT/packaging/debian/postrm" "$DEB_DIR/DEBIAN/"

# Set proper permissions on control scripts
chmod 755 "$DEB_DIR/DEBIAN/postinst"
chmod 755 "$DEB_DIR/DEBIAN/prerm" 
chmod 755 "$DEB_DIR/DEBIAN/postrm"
chmod 644 "$DEB_DIR/DEBIAN/control"

# MARK: Create conffiles (mark config files)
cat > "$DEB_DIR/DEBIAN/conffiles" << EOF
/etc/finguard/config.yaml
/etc/finguard/services.yaml
/etc/finguard/wireguard.yaml
/etc/finguard/update.yaml
EOF

# MARK: Calculate installed size
INSTALLED_SIZE=$(du -sk "$DEB_DIR" | cut -f1)
echo "Installed-Size: $INSTALLED_SIZE" >> "$DEB_DIR/DEBIAN/control"

# MARK: Build the .deb package
echo "Building .deb package..."
cd "$BUILD_DIR"
dpkg-deb --build "${PACKAGE_NAME}_${VERSION}_${ARCH}"

# MARK: Verify the package
echo "Verifying package..."
DEB_FILE="${BUILD_DIR}/${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"
if [ -f "$DEB_FILE" ]; then
    echo ""
    echo "✅ Package built successfully!"
    echo "📦 Package: $DEB_FILE"
    echo "📊 Size: $(du -h "$DEB_FILE" | cut -f1)"
    echo "🏷️ Version: $VERSION"
    echo ""
    echo "Package contents:"
    dpkg-deb --contents "$DEB_FILE" | head -20
    echo ""
    echo "To install:"
    echo "  sudo dpkg -i $DEB_FILE"
    echo ""
    echo "To install with dependency resolution:"
    echo "  sudo apt install $DEB_FILE"
    echo ""
    echo "After installation:"
    echo "  1. Edit /etc/finguard/config.yaml (set admin_token)"
    echo "  2. sudo systemctl start finguard"
    echo "  3. Open http://localhost:10000"
    echo ""
else
    echo "❌ Package build failed!"
    exit 1
fi