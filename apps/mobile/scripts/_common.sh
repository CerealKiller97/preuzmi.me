#!/usr/bin/env bash
# Shared helpers for the mobile build scripts. Sourced, not run directly.

set -euo pipefail

# Repo layout: this file is apps/mobile/scripts/_common.sh
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MOBILE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$MOBILE_DIR/dist"

# --- locate the Flutter SDK ---------------------------------------------------
# Prefer whatever is on PATH; otherwise fall back to a Homebrew install (this
# machine's default). Override by exporting FLUTTER before running.
if [[ -z "${FLUTTER:-}" ]]; then
  if command -v flutter >/dev/null 2>&1; then
    FLUTTER="$(command -v flutter)"
  elif [[ -x /opt/homebrew/bin/flutter ]]; then
    FLUTTER="/opt/homebrew/bin/flutter"
  else
    echo "error: Flutter SDK not found. Install it or export FLUTTER=/path/to/flutter" >&2
    exit 1
  fi
fi

# --- Android toolchain (only needed for the APK) ------------------------------
# Fill in sensible macOS defaults when the caller hasn't configured them, so the
# script works out of the box while still honouring an existing environment.
if [[ -z "${ANDROID_HOME:-}" && -d "$HOME/Library/Android/sdk" ]]; then
  export ANDROID_HOME="$HOME/Library/Android/sdk"
fi
if [[ -z "${JAVA_HOME:-}" ]]; then
  for jbr in "$HOME/Applications/Android Studio.app/Contents/jbr/Contents/Home" \
             "/Applications/Android Studio.app/Contents/jbr/Contents/Home"; do
    if [[ -x "$jbr/bin/java" ]]; then
      export JAVA_HOME="$jbr"
      break
    fi
  done
fi

# App version, shaped "<name>[+<number>]" (e.g. "1.7.0+10700" or "1.7.0"):
#   1. an explicit APP_VERSION wins — CI sets it from the pushed vX.Y.Z tag so
#      the build carries the release version, not whatever pubspec.yaml holds;
#   2. otherwise the pubspec value (the local-dev fallback).
app_version() {
  if [[ -n "${APP_VERSION:-}" ]]; then
    echo "${APP_VERSION}"
    return
  fi
  local v
  v="$(grep '^version:' "$MOBILE_DIR/pubspec.yaml" | awk '{print $2}' | tr -d '\r')"
  echo "${v:-0.0.0}"
}

# Flutter --build-name: the semver before '+', e.g. "1.7.0".
build_name() { app_version | cut -d'+' -f1; }

# Flutter --build-number: the integer after '+', or 1 when the version has none.
build_number() {
  local v
  v="$(app_version)"
  [[ "$v" == *+* ]] && echo "${v##*+}" || echo "1"
}

# Version used in the artifact filenames — the semver only, so the outputs are
# preuzmi-<version>.apk / preuzmi-<version>-unsigned.ipa (as README/CHANGELOG say).
version_tag() { build_name; }

log() { printf '\033[1;36m▸ %s\033[0m\n' "$*"; }
