#!/usr/bin/env sh
# Builds TroopersTransfer for the current OS (Linux or macOS) into ./dist.
# Linux needs: gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev
# (Debian/Ubuntu package names; see "Building on Linux" in the README for
# Fedora and Arch).
# macOS needs: Xcode command line tools.
set -eu
cd "$(dirname "$0")"
VERSION="${VERSION:-1.2.0}"
mkdir -p dist
CGO_ENABLED=1 go build -trimpath \
  -ldflags "-s -w -X github.com/troopers/transfer-service/ui.Version=${VERSION}" \
  -o dist/TroopersTransfer .
ls -lh dist/TroopersTransfer
