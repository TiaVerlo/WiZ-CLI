#!/bin/bash
# -*- coding: utf-8 -*-

clear

cd "$(cd -- "$(dirname -- "$0")" && pwd)"

# define output directory variable
dist="../dist"

# ensure output directory exists
mkdir -p "$dist"

# -- Intel / amd64 build
echo "building Intel (amd64) binary..."
GOOS=darwin GOARCH=amd64 go build -o "$dist/wiz-cli-macos-amd64" wiz-cli.go
chmod +x "$dist/wiz-cli-macos-amd64"
codesign --force --deep --sign - "$dist/wiz-cli-macos-amd64"

# --- Apple Silicon / arm64 build
echo "building Apple Silicon (arm64) binary..."
GOOS=darwin GOARCH=arm64 go build -o "$dist/wiz-cli-macos-arm64" wiz-cli.go
chmod +x "$dist/wiz-cli-macos-arm64"
codesign --force --deep --sign - "$dist/wiz-cli-macos-arm64"

# -- universal binary
echo "creating universal binary..."
lipo -create -output "$dist/wiz-cli-macos" "$dist/wiz-cli-macos-amd64" "$dist/wiz-cli-macos-arm64"
chmod +x "$dist/wiz-cli-macos"
codesign --force --deep --sign - "$dist/wiz-cli-macos"

echo "ENTER to exit..."
read