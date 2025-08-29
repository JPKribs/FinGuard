#!/bin/bash
set -e

# MARK: FinGuard Debian Package Builder
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
PACKAGE_NAME="finguard"

# MARK: Detect system architecture
detect_architecture() {
    local go_arch=""
    local deb_arch=""
    
    case "$(uname -m)" in
        x86_64)
            go_arch="amd64"
            deb_arch="amd64"
            ;;
        aarch64|arm64)
            go_arch="arm64"
            deb_arch="arm64"
            ;;
        armv7l)
            go_arch="arm"
            deb_arch="armhf"
            ;;
        i386|i686)
            go_arch="386"
            deb_arch="i386"
            ;;
        *)
            echo "Unsupported architecture: $(uname -m)" >&2
            exit 1
            ;;
    esac
    
    echo "$go_arch,$deb_arch"
}

ARCHS=($(detect_architecture))
GO_ARCH="${ARCHS[0]}"
DEB_ARCH="${ARCHS[1]}"

echo "Detected architecture: $(uname -m) -> Go: $GO_ARCH, Debian: $DEB_ARCH"

# MARK: Get version from main.go or git tag
get_version() {
    local version=""
    
    if [ -f "$PROJECT_ROOT/main.go" ]; then
        version=$(grep -E 'Version\s*=\s*"[^"]*"' "$PROJECT_ROOT/main.go" | sed 's/.*Version\s*=\s*"\([^"]*\)".*/\1/')
    fi
    
    if [ -z "$version" ] && command -v git >/dev/null 2>&1; then
        if git -C "$PROJECT_ROOT" describe --tags --exact-match 2>/dev/null; then
            version=$(git -C "$PROJECT_ROOT" describe --tags --exact-match 2>/dev/null | sed 's/^v//')
        fi
    fi
    
    if [ -z "$version" ] && command -v git >/dev/null 2>&1; then
        if git -C "$PROJECT_ROOT" describe --tags 2>/dev/null; then
            version=$(git -C "$PROJECT_ROOT" describe --tags 2>/dev/null | sed 's/^v//' | sed 's/-g[a-f0-9]*$//')
        fi
    fi
    
    if [ -z "$version" ]; then
        version="1.0.0-dev"
        echo "Warning: Could not determine version, using default: $version" >&2
    fi
    
    echo "$version"
}

VERSION=$(get_version)
echo "Building FinGuard Debian package..."
echo "Project root: $PROJECT_ROOT"
echo "Version: $VERSION"

# MARK: Clean and create build directories
BUILD_DIR="$PROJECT_ROOT/build/debian"
DEB_DIR="$BUILD_DIR/${PACKAGE_NAME}_${VERSION}_${DEB_ARCH}"

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
echo "Building FinGuard binary with version $VERSION for $GO_ARCH..."
cd "$PROJECT_ROOT"

mkdir -p bin
if ! CGO_ENABLED=0 GOOS=linux GOARCH="$GO_ARCH" go build -ldflags "-X main.Version=$VERSION" -o "bin/finguard" ./cmd/finguard; then
    echo "Build failed. Make sure you have Go installed and the project compiles."
    exit 1
fi

# MARK: Copy files to package structure
echo "Copying binary..."
cp "$PROJECT_ROOT/bin/finguard" "$DEB_DIR/usr/local/bin/"
chmod 755 "$DEB_DIR/usr/local/bin/finguard"

echo "Copying web interface..."
if [ -d "$PROJECT_ROOT/web" ]; then
    cp -r "$PROJECT_ROOT/web/"* "$DEB_DIR/usr/local/share/finguard/web/"
    find "$DEB_DIR/usr/local/share/finguard/web" -type f -exec chmod 644 {} \;
    find "$DEB_DIR/usr/local/share/finguard/web" -type d -exec chmod 755 {} \;
else
    echo "Warning: No web directory found at $PROJECT_ROOT/web"
fi

echo "Copying configuration files..."
if [ -f "$PROJECT_ROOT/config-production.yaml" ]; then
    cp "$PROJECT_ROOT/config-production.yaml" "$DEB_DIR/etc/finguard/config.yaml"
    chmod 640 "$DEB_DIR/etc/finguard/config.yaml"
elif [ -f "$PROJECT_ROOT/config.yaml" ]; then
    cp "$PROJECT_ROOT/config.yaml" "$DEB_DIR/etc/finguard/"
    chmod 640 "$DEB_DIR/etc/finguard/config.yaml"
fi

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
if [ -f "$PROJECT_ROOT/packaging/debian/finguard.service" ]; then
    cp "$PROJECT_ROOT/packaging/debian/finguard.service" "$DEB_DIR/etc/systemd/system/"
    chmod 644 "$DEB_DIR/etc/systemd/system/finguard.service"
else
    echo "Warning: systemd service file not found"
fi

# MARK: Create control file with dynamic architecture
echo "Creating Debian control file..."
cat > "$DEB_DIR/DEBIAN/control" << EOF
Package: finguard
Version: $VERSION
Section: net
Priority: optional
Architecture: $DEB_ARCH
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
for file in postinst prerm postrm; do
    if [ -f "$PROJECT_ROOT/packaging/debian/$file" ]; then
        cp "$PROJECT_ROOT/packaging/debian/$file" "$DEB_DIR/DEBIAN/"
        chmod 755 "$DEB_DIR/DEBIAN/$file"
    fi
done
chmod 644 "$DEB_DIR/DEBIAN/control"

# MARK: Create conffiles
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
dpkg-deb --build "${PACKAGE_NAME}_${VERSION}_${DEB_ARCH}"

# MARK: Verify the package
echo "Verifying package..."
DEB_FILE="${BUILD_DIR}/${PACKAGE_NAME}_${VERSION}_${DEB_ARCH}.deb"
if [ -f "$DEB_FILE" ]; then
    echo ""
    echo "Package built successfully!"
    echo "Package: $DEB_FILE"
    echo "Size: $(du -h "$DEB_FILE" | cut -f1)"
    echo "Version: $VERSION"
    echo "Architecture: $DEB_ARCH"
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
    echo "Package build failed!"
    exit 1
fi