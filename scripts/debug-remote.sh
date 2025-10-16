#!/bin/bash
# Helper script for remote debugging pi-player
# Usage: ./scripts/debug-remote.sh [user@host]

set -e

# Default to environment variable if no argument provided
TARGET="${1:-${PI_PLAYER_DEBUG_HOST}}"

if [ -z "$TARGET" ]; then
    echo "Usage: $0 user@host"
    echo "   Or set PI_PLAYER_DEBUG_HOST environment variable"
    echo ""
    echo "Example:"
    echo "  $0 sandtonvisuals@192.168.20.80"
    echo "  export PI_PLAYER_DEBUG_HOST=sandtonvisuals@192.168.20.80"
    echo "  $0"
    exit 1
fi

echo "Setting up remote debugging for $TARGET..."
echo ""
echo "Commands to run on remote machine ($TARGET):"
echo "  systemctl --user stop pi-player"
echo "  dlv exec ~/.local/bin/pi-player --headless --listen=:2345 --api-version=2 --accept-multiclient"
echo ""
echo "Opening SSH tunnel (port 2345)..."
echo "Press Ctrl+C to close the tunnel when done debugging."
echo ""

# SSH with port forwarding and keep connection open
ssh -L 2345:localhost:2345 "$TARGET"
