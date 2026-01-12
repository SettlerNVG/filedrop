#!/bin/bash
# Run relay with ngrok tunnel

set -e

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
RELAY_PORT=9000

echo "🚀 Starting FileDrop Public Relay with ngrok..."

# Check ngrok
if ! command -v ngrok &> /dev/null; then
    echo "❌ ngrok not found. Install with: brew install ngrok"
    echo "   Then authenticate: ngrok config add-authtoken <your-token>"
    echo "   Get token at: https://dashboard.ngrok.com/get-started/your-authtoken"
    exit 1
fi

# Kill any existing ngrok
pkill -f "ngrok tcp" 2>/dev/null || true
sleep 1

# Start relay in background
echo "📡 Starting relay server..."
cd "$REPO_DIR"
go run ./cmd/relay -port $RELAY_PORT > /dev/null 2>&1 &
RELAY_PID=$!
sleep 2

# Start ngrok
echo "🌐 Starting ngrok tunnel..."
ngrok tcp $RELAY_PORT > /dev/null 2>&1 &
NGROK_PID=$!

# Wait for ngrok API to be ready
echo "⏳ Waiting for ngrok..."
NGROK_URL=""
for i in {1..30}; do
    NGROK_URL=$(curl -s http://localhost:4040/api/tunnels 2>/dev/null | grep -o '"public_url":"tcp://[^"]*"' | cut -d'"' -f4 | sed 's|tcp://||' || true)
    if [ -n "$NGROK_URL" ]; then
        break
    fi
    sleep 1
done

if [ -z "$NGROK_URL" ]; then
    echo "❌ Failed to get ngrok URL"
    echo "Try running manually: ngrok tcp 9000"
    kill $RELAY_PID 2>/dev/null
    exit 1
fi

echo ""
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║              🎉 FileDrop Public Relay Ready!                   ║"
echo "╠════════════════════════════════════════════════════════════════╣"
printf "║  Public Address: %-44s║\n" "$NGROK_URL"
echo "╠════════════════════════════════════════════════════════════════╣"
echo "║  Share with users:                                             ║"
printf "║    filedrop -relay %s send <file>\n" "$NGROK_URL"
printf "║    filedrop-tui -relay %s\n" "$NGROK_URL"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

# Update relay-address.txt
echo "$NGROK_URL" > "$REPO_DIR/relay-address.txt"
echo "📝 Updated relay-address.txt with: $NGROK_URL"

# Push to GitHub
if [ -d "$REPO_DIR/.git" ]; then
    cd "$REPO_DIR"
    git add relay-address.txt 2>/dev/null || true
    git commit -m "chore: update relay address" 2>/dev/null || true
    git push 2>/dev/null && echo "✅ Pushed to GitHub - clients will auto-connect!" || echo "⚠️  Could not push (maybe no changes)"
fi

echo ""
echo "Press Ctrl+C to stop..."

cleanup() {
    echo ""
    echo "🛑 Stopping..."
    kill $RELAY_PID 2>/dev/null
    pkill -f "ngrok tcp" 2>/dev/null || true
    echo "localhost:9000" > "$REPO_DIR/relay-address.txt"
    exit 0
}
trap cleanup INT TERM

# Keep running
while true; do
    sleep 1
done
