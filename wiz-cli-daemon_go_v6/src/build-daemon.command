#!/bin/bash
# -*- coding: utf-8 -*-

set -euo pipefail

clear

# switch to directory containing this script
cd "$(cd -- "$(dirname -- "$0")" && pwd)"

# define output directory
dist="../dist"
name="wiz-cli-d"

# ensure output directory exists
mkdir -p "$dist"

# --- Intel / x86-64 build ------------------------------------------------------------------------
echo "building Intel (x86-64) binary..."
GOOS=darwin GOARCH=amd64 go build \
    -o "$dist/${name}-macos-x86-64" \
    wiz-cli-daemon.go

chmod +x "$dist/${name}-macos-x86-64"

codesign \
    --force \
    --sign - \
    "$dist/${name}-macos-x86-64"


# --- Apple Silicon / arm64 build -----------------------------------------------------------------
echo "building Apple Silicon (arm64) binary..."
GOOS=darwin GOARCH=arm64 go build \
    -o "$dist/${name}-macos-arm64" \
    wiz-cli-daemon.go

chmod +x "$dist/${name}-macos-arm64"

codesign \
    --force \
    --sign - \
    "$dist/${name}-macos-arm64"


# --- Universal binary ----------------------------------------------------------------------------
echo "creating universal binary..."

lipo -create \
    -output "$dist/${name}-macos" \
    "$dist/${name}-macos-x86-64" \
    "$dist/${name}-macos-arm64"

chmod +x "$dist/${name}-macos"

codesign \
    --force \
    --sign - \
    "$dist/${name}-macos"


echo
echo "build complete:"
file "$dist/${name}-macos"
lipo -archs "$dist/${name}-macos"

echo
read -r -p "ENTER to exit..."