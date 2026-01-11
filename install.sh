#!/bin/bash
# FileDrop Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/SettlerNVG/filedrop/main/install.sh | bash

set -e

REPO="SettlerNVG/filedrop"
INSTALL_DIR="/usr/local/bin"

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
        EXT=""
        ;;
    darwin)
        PLATFORM="darwin"
        EXT=""
        ;;
    mingw*|msys*|cygwin*)
        PLATFORM="windows"
        EXT=".exe"
        ;;
    *)
        echo -e "${RED}Unsupported OS: $OS${NC}"
        exit 1
        ;;
esac

echo -e "Detected: ${YELLOW}${PLATFORM}/${ARCH}${NC}"

# Create temp directory
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

download_binary() {
    local name=$1
    local output=$2
    local url="https://github.com/${REPO}/releases/latest/download/${name}"
    
    echo -e "Downloading: ${YELLOW}${name}${NC}"
    
    if command -v curl &> /dev/null; then
        curl -fsSL "$url" -o "$TMP_DIR/$output" 2>/dev/null || return 1
    elif command -v wget &> /dev/null; then
        wget -q "$url" -O "$TMP_DIR/$output" 2>/dev/null || return 1
    else
        echo -e "${RED}Error: curl or wget required${NC}"
        exit 1
    fi
    
    chmod +x "$TMP_DIR/$output"
    return 0
}

install_binary() {
    local name=$1
    if [ -w "$INSTALL_DIR" ]; then
        mv "$TMP_DIR/$name" "$INSTALL_DIR/$name"
    else
        sudo mv "$TMP_DIR/$name" "$INSTALL_DIR/$name"
    fi
}

# Download CLI client
CLI_BINARY="filedrop-${PLATFORM}-${ARCH}${EXT}"
CLI_NAME="filedrop${EXT}"
if download_binary "$CLI_BINARY" "$CLI_NAME"; then
    echo -e "${GREEN}✓${NC} CLI client downloaded"
else
    echo -e "${RED}✗${NC} Failed to download CLI client"
fi

# Download TUI client
TUI_BINARY="filedrop-tui-${PLATFORM}-${ARCH}${EXT}"
TUI_NAME="filedrop-tui${EXT}"
if download_binary "$TUI_BINARY" "$TUI_NAME"; then
    echo -e "${GREEN}✓${NC} TUI client downloaded"
else
    echo -e "${YELLOW}!${NC} TUI client not available (optional)"
fi

# Install
echo ""
if [ ! -w "$INSTALL_DIR" ]; then
    echo -e "${YELLOW}Need sudo to install to $INSTALL_DIR${NC}"
fi

[ -f "$TMP_DIR/$CLI_NAME" ] && install_binary "$CLI_NAME"
[ -f "$TMP_DIR/$TUI_NAME" ] && install_binary "$TUI_NAME"

# Verify installation
echo ""
echo -e "${GREEN}✅ FileDrop installed successfully!${NC}"
echo ""
echo "Commands:"
echo ""
echo -e "  ${YELLOW}filedrop${NC} - CLI client (quick file transfer by code)"
echo "    filedrop send <file>        # Send a file, get code"
echo "    filedrop receive <code>     # Receive by code"
echo ""
echo -e "  ${YELLOW}filedrop-tui${NC} - Interactive TUI (see online users)"
echo "    filedrop-tui                # Launch interactive mode"
echo ""
echo "Examples:"
echo "  filedrop -relay server:9000 send myfile.zip"
echo "  filedrop-tui -relay server:9000"
echo ""
