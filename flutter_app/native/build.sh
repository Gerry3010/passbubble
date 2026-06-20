#!/usr/bin/env bash
# Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU Affero General Public License as published by
# the Free Software Foundation, either version 3 of the License, or (at your
# option) any later version. See <https://www.gnu.org/licenses/>.
#
# Builds the Passbubble hybrid-KEM c-shared library (a thin wrapper around
# backend/pkg/crypto) for the host platform — and, when the Android NDK is
# present, for every Android ABI. Output goes where Flutter's per-platform build
# expects it. Wire format is identical across CLI / backend / extension / app.
#
# Usage:
#   ./build.sh            # host platform (Linux/macOS/Windows desktop)
#   ./build.sh android    # all Android ABIs (needs ANDROID_NDK_HOME)
#
# iOS/macOS: produce a static lib / xcframework and link it in Xcode (see README).
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/passbubble_crypto" && pwd)"
cd "$DIR"
NAME="passbubble_crypto"

build_host() {
  mkdir -p build
  local out ext
  case "$(uname -s)" in
    Linux)  ext="so" ;;
    Darwin) ext="dylib" ;;
    MINGW*|MSYS*|CYGWIN*) ext="dll" ;;
    *) echo "unsupported host"; exit 1 ;;
  esac
  out="build/lib${NAME}.${ext}"
  echo "→ $out"
  CGO_ENABLED=1 go build -buildmode=c-shared -o "$out" .
}

build_android() {
  : "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to the NDK path}"
  local tc="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/bin"
  declare -A abis=(
    [arm64-v8a]="aarch64-linux-android21-clang"
    [armeabi-v7a]="armv7a-linux-androideabi21-clang"
    [x86_64]="x86_64-linux-android21-clang"
  )
  for abi in "${!abis[@]}"; do
    local dest="../../android/app/src/main/jniLibs/$abi"
    mkdir -p "$dest"
    echo "→ $dest/lib${NAME}.so"
    CGO_ENABLED=1 CC="$tc/${abis[$abi]}" \
      GOOS=android GOARCH="$([[ $abi == arm64-v8a ]] && echo arm64 || { [[ $abi == x86_64 ]] && echo amd64 || echo arm; })" \
      go build -buildmode=c-shared -o "$dest/lib${NAME}.so" .
  done
}

case "${1:-host}" in
  host) build_host ;;
  android) build_android ;;
  *) echo "usage: $0 [host|android]"; exit 1 ;;
esac
echo "done."
