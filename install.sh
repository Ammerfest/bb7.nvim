#!/bin/sh
# install.sh — build the bb7 binary from source.
# Called automatically by lazy.nvim's build step.
set -e

if ! command -v go >/dev/null 2>&1; then
  echo "Error: Go is not installed."
  echo "Install Go from https://go.dev/dl/ and run :Lazy build bb7"
  exit 1
fi

go build -o bb7 ./cmd/bb7
echo "Built bb7 from source."
