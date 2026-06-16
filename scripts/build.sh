#!/usr/bin/env bash
# VinoLlama build script for Linux/macOS
# Usage: ./scripts/build.sh [--skip-tests] [--skip-frontend] [--skip-desktop] [--clean]

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SKIP_TESTS=false
SKIP_FRONTEND=false
SKIP_DESKTOP=false
CLEAN=false

for arg in "$@"; do
    case "$arg" in
        --skip-tests) SKIP_TESTS=true ;;
        --skip-frontend) SKIP_FRONTEND=true ;;
        --skip-desktop) SKIP_DESKTOP=true ;;
        --clean) CLEAN=true ;;
        *) echo "Unknown option: $arg"; exit 1 ;;
    esac
done

echo "=== VinoLlama Build ==="
echo ""

# Clean
if [ "$CLEAN" = true ]; then
    echo "[clean] Removing build artifacts..."
    rm -rf "$ROOT/desktop/build/bin" "$ROOT/desktop/frontend/dist"
    echo "[clean] Done."
fi

# Backend tests
if [ "$SKIP_TESTS" = false ]; then
    echo "[test] Running Go tests..."
    cd "$ROOT"
    go test ./...
    echo "[test] Go tests passed."
    echo ""
fi

# Frontend checks
if [ "$SKIP_FRONTEND" = false ]; then
    echo "[frontend] Installing dependencies..."
    cd "$ROOT/desktop/frontend"
    npm install

    echo "[frontend] Running typecheck..."
    npm run typecheck

    echo "[frontend] Running tests..."
    npm test

    echo "[frontend] Building frontend..."
    npm run build

    echo "[frontend] Frontend checks passed."
    echo ""
fi

# Desktop build (Wails)
if [ "$SKIP_DESKTOP" = false ]; then
    echo "[desktop] Building Wails desktop app..."
    cd "$ROOT/desktop"
    wails build
    echo "[desktop] Desktop build complete."
    echo ""

    if [ -f "$ROOT/desktop/build/bin/VinoLlama" ] || [ -f "$ROOT/desktop/build/bin/VinoLlama.exe" ]; then
        echo "Output: $(ls -lh "$ROOT/desktop/build/bin/VinoLlama"* 2>/dev/null | head -1)"
    fi
fi

echo ""
echo "=== Build Complete ==="
