#!/bin/bash
# Run Neovim plugin tests using plenary.nvim
# Requires: plenary.nvim installed in Neovim

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="$(dirname "$SCRIPT_DIR")"

# Isolate from user config (after/plugin, custom init, etc.) to keep tests deterministic.
TEST_CONFIG_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_CONFIG_DIR"' EXIT
export XDG_CONFIG_HOME="$TEST_CONFIG_DIR"

# Check if plenary is available
if ! nvim --headless -c "lua require('plenary')" -c "qa" 2>/dev/null; then
    echo "ERROR: plenary.nvim is required but not found"
    echo "Install it with your package manager (e.g., lazy.nvim, packer)"
    exit 1
fi

echo "Running bb7 plugin tests..."
echo "================================"

# Run tests
nvim --headless \
    -u "$SCRIPT_DIR/minimal_init.lua" \
    -c "PlenaryBustedDirectory $SCRIPT_DIR/ {minimal_init = '$SCRIPT_DIR/minimal_init.lua', sequential = true}"

echo "================================"
echo "Tests completed!"
