#!/bin/bash
# FileDrop Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/SettlerNVG/filedrop/main/install.sh | bash

set -e

REPO="SettlerNVG/filedrop"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="filedrop"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}"
echo "  _____ _ _      ____                  "
echo " |  ___(_) | ___|  _ \ _ __ ___  _ __  "
echo " | |_  | | |/ _ \ | | | '__/ _ \| '_ \ "
echo " |  _| | | |  __/ |_| | | | (_) | |_) |"
echo " |_|   |_|_|\___|____/|_|  \___/| .__/ "
echo "                                |_|    "
echo -e "${NC}"
echo "Installing FileDrop..."
echo ""

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

case "$OS" in
    linux)
        PLATFORM="linux"
        ;;
    darwin)
        PLATFORM="darwin"
        ;;
    mingw*|msys*|cygwin*)
        PLATFORM="windows"
        BINARY_NAME="filedrop.exe"
        ;;
    *)
        echo -e "${RED}Unsupported OS: $OS${NC}"
        exit 1
        ;;
esac

BINARY="filedrop-${PLATFORM}-${ARCH}"
if [ "$PLATFORM" = "windows" ]; then
    BINARY="${BINARY}.exe"
fi

echo -e "Detected: ${YELLOW}${PLATFORM}/${ARCH}${NC}"

# Get latest release URL
RELEASE_URL="https://github.com/${REPO}/releases/latest/download/${BINARY}"

echo -e "Downloading from: ${YELLOW}${RELEASE_URL}${NC}"

# Create temp directory
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

# Download binary
if command -v curl &> /dev/null; then
    curl -fsSL "$RELEASE_URL" -o "$TMP_DIR/$BINARY_NAME"
elif command -v wget &> /dev/null; then
    wget -q "$RELEASE_URL" -O "$TMP_DIR/$BINARY_NAME"
else
    echo -e "${RED}Error: curl or wget required${NC}"
    exit 1
fi

# Make executable
chmod +x "$TMP_DIR/$BINARY_NAME"

# Install
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
else
    echo -e "${YELLOW}Need sudo to install to $INSTALL_DIR${NC}"
    sudo mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
fi

# Verify installation
if command -v filedrop &> /dev/null; then
    echo ""
    echo -e "${GREEN}✅ FileDrop installed successfully!${NC}"
    echo ""
    echo "Usage:"
    echo "  filedrop send <file>      # Send a file"
    echo "  filedrop receive <code>   # Receive a file"
    echo ""
    echo "Example:"
    echo "  filedrop -relay your-server:9000 send myfile.zip"
else
    echo -e "${RED}Installation failed${NC}"
    exit 1
fi
