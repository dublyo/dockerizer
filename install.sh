#!/bin/sh
# Dockerizer Installer
# https://dockerizer.dev
#
# Usage:
#   curl -fsSL https://dockerizer.dev/install.sh | sh
#
# Options (via environment variables):
#   INSTALL_DIR   Installation directory (default: /usr/local/bin)
#   VERSION       Specific version to install (default: latest)

set -e

# Configuration
GITHUB_REPO="dublyo/dockerizer"
BINARY_NAME="dockerizer"
DEFAULT_INSTALL_DIR="/usr/local/bin"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
info() {
    printf "${BLUE}[INFO]${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}[OK]${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1" >&2
    exit 1
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux)
            OS="linux"
            ;;
        darwin)
            OS="darwin"
            ;;
        mingw*|msys*|cygwin*)
            OS="windows"
            ;;
        *)
            error "Unsupported operating system: $OS"
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            error "Unsupported architecture: $ARCH"
            ;;
    esac

    PLATFORM="${OS}_${ARCH}"
    info "Detected platform: $PLATFORM"
}

# Get the latest version from GitHub
get_latest_version() {
    if [ -n "$VERSION" ]; then
        info "Using specified version: $VERSION"
        return
    fi

    info "Fetching latest version..."
    VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/' || echo "")

    if [ -z "$VERSION" ]; then
        warn "Could not determine latest version, using 'latest'"
        VERSION="latest"
    else
        info "Latest version: v$VERSION"
    fi
}

# Download and install
install() {
    INSTALL_DIR="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

    # Check if we need sudo
    SUDO=""
    if [ ! -w "$INSTALL_DIR" ]; then
        if command -v sudo >/dev/null 2>&1; then
            SUDO="sudo"
            info "Installation directory requires elevated privileges"
        else
            error "Cannot write to $INSTALL_DIR and sudo is not available"
        fi
    fi

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT

    # Construct download URL
    if [ "$VERSION" = "latest" ]; then
        DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/${BINARY_NAME}_${PLATFORM}.tar.gz"
    else
        DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/${BINARY_NAME}_${PLATFORM}.tar.gz"
    fi

    info "Downloading from: $DOWNLOAD_URL"

    # Download
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/dockerizer.tar.gz" || error "Download failed"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "$DOWNLOAD_URL" -O "$TMP_DIR/dockerizer.tar.gz" || error "Download failed"
    else
        error "Neither curl nor wget found. Please install one of them."
    fi

    # Extract
    info "Extracting..."
    tar -xzf "$TMP_DIR/dockerizer.tar.gz" -C "$TMP_DIR" || error "Extraction failed"

    # Install
    info "Installing to $INSTALL_DIR..."
    $SUDO mkdir -p "$INSTALL_DIR"
    $SUDO mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
    $SUDO chmod +x "$INSTALL_DIR/$BINARY_NAME"

    success "Dockerizer installed successfully!"
}

# Verify installation
verify() {
    if command -v dockerizer >/dev/null 2>&1; then
        success "Verification successful"
        echo ""
        dockerizer version
        echo ""
        info "Get started with: dockerizer init"
    else
        warn "Dockerizer installed but not in PATH"
        info "Add $INSTALL_DIR to your PATH or run: $INSTALL_DIR/dockerizer"
    fi
}

# Main
main() {
    echo ""
    echo "  Dockerizer Installer"
    echo "  https://dockerizer.dev"
    echo ""

    detect_platform
    get_latest_version
    install
    verify

    echo ""
    success "Installation complete!"
    echo ""
    echo "  Quick Start:"
    echo "    dockerizer init          # Interactive setup"
    echo "    dockerizer .             # Auto-detect and generate"
    echo "    dockerizer plan .        # Preview build plan"
    echo ""
    echo "  Documentation: https://dockerizer.dev"
    echo ""
}

main "$@"
