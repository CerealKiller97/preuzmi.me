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

# App version from pubspec (e.g. "1.0.0+1" -> "1.0.0+1"); '+' is filename-safe
# once turned into '_'.
app_version() {
  local v
  v="$(grep '^version:' "$MOBILE_DIR/pubspec.yaml" | awk '{print $2}' | tr -d '\r')"
  echo "${v:-0.0.0}"
}

version_tag() {
  app_version | tr '+' '_'
}

log() { printf '\033[1;36m▸ %s\033[0m\n' "$*"; }
