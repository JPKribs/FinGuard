#!/bin/bash
set -e

# MARK: FinGuard Debian Package Builder
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PACKAGE_NAME="finguard"
VERSION="1.0.0-beta2"
ARCH="amd64"

echo "Building FinGuard Debian package..."
echo "Project root: $PROJECT_ROOT"

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

# MARK: Build the binary
echo "Building FinGuard binary..."
cd "$PROJECT_ROOT"
if [ ! -f "bin/finguard" ]; then
    echo "Binary not found, building..."
    make build || {
        echo "Build failed. Make sure you have Go installed and run 'make build' first."
        exit 1
    }
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

chmod 644 "$DEB_DIR/etc/finguard/services.yaml"
chmod 600 "$DEB_DIR/etc/finguard/wireguard.yaml"

echo "Copying systemd service..."
cp "$PROJECT_ROOT/packaging/debian/finguard.service" "$DEB_DIR/etc/systemd/system/"
chmod 644 "$DEB_DIR/etc/systemd/system/finguard.service"

# MARK: Copy Debian control files
echo "Creating Debian control files..."
cp "$PROJECT_ROOT/packaging/debian/control" "$DEB_DIR/DEBIAN/"
cp "$PROJECT_ROOT/packaging/debian/postinst" "$DEB_DIR/DEBIAN/"
cp "$PROJECT_ROOT/packaging/debian/prerm" "$DEB_DIR/DEBIAN/"
cp "$PROJECT_ROOT/packaging/debian/postrm" "$DEB_DIR/DEBIAN/"

# Set proper permissions on control scripts
chmod 755 "$DEB_DIR/DEBIAN/postinst"
chmod 755 "$DEB_DIR/DEBIAN/prerm" 
chmod 755 "$DEB_DIR/DEBIAN/postrm"
chmod 644 "$DEB_DIR/DEBIAN/control"

# MARK: Create conffiles (mark config files)
echo "/etc/finguard/config.yaml" > "$DEB_DIR/DEBIAN/conffiles"
echo "/etc/finguard/services.yaml" >> "$DEB_DIR/DEBIAN/conffiles"
echo "/etc/finguard/wireguard.yaml" >> "$DEB_DIR/DEBIAN/conffiles"

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