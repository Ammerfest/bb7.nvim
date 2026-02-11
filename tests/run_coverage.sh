#!/bin/bash
# Run Go coverage and store artifacts under .tmp/coverage/.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
OUT_DIR="$REPO_ROOT/.tmp/coverage"

mkdir -p "$OUT_DIR"

echo "Running Go coverage..."

go test ./... -coverprofile="$OUT_DIR/coverage.out"
go tool cover -func="$OUT_DIR/coverage.out" > "$OUT_DIR/summary.txt"
go tool cover -html="$OUT_DIR/coverage.out" -o "$OUT_DIR/coverage.html"

echo "Coverage artifacts written to: $OUT_DIR"
echo "- $OUT_DIR/coverage.out"
echo "- $OUT_DIR/summary.txt"
echo "- $OUT_DIR/coverage.html"

echo
cat "$OUT_DIR/summary.txt"
