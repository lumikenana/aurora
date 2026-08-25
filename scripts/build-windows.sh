#!/bin/bash
# Cross-compile GameAI for Windows from macOS

set -e

echo "=== Building GameAI for Windows ==="

# Check if mingw-w64 is installed
if ! command -v x86_64-w64-mingw32-gcc &> /dev/null; then
    echo "Error: x86_64-w64-mingw32-gcc not found."
    echo "Please install it with: brew install mingw-w64"
    exit 1
fi

# Check if wails is installed
if ! command -v wails &> /dev/null; then
    echo "Error: wails not found."
    echo "Please install it with: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
fi

echo ">> Using compiler: $(x86_64-w64-mingw32-gcc --version | head -1)"

# Clean previous build
rm -rf build

# Build
echo ">> Starting build..."
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc wails build

echo ""
echo "=== Build Complete ==="
echo "Output: build/bin/GameAI.exe"
echo "File size: $(du -h build/bin/GameAI.exe | cut -f1)"
