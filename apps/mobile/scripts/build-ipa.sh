#!/usr/bin/env bash
#
# Build a production (release) iOS build for the preuzmi.me mobile app and copy
# the resulting .ipa into apps/mobile/dist/. macOS + Xcode required.
#
# Two modes:
#   scripts/build-ipa.sh           # UNSIGNED .ipa (no Apple account needed).
#                                  # Install by sideloading (Sideloadly / AltStore),
#                                  # which re-signs it with your Apple ID.
#   SIGNED=1 scripts/build-ipa.sh  # Signed .ipa via `flutter build ipa`.
#                                  # Requires signing set up in Xcode (a team +
#                                  # provisioning). Honors EXPORT_METHOD
#                                  # (development | ad-hoc | app-store).
#
# Env overrides: FLUTTER, EXPORT_METHOD (default: development).

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/_common.sh"

if [[ "$(uname)" != "Darwin" ]]; then
  echo "error: iOS builds require macOS with Xcode." >&2
  exit 1
fi

cd "$MOBILE_DIR"
mkdir -p "$DIST_DIR"
version="$(version_tag)"
name="$(build_name)"
number="$(build_number)"

log "Flutter: $FLUTTER"
log "Version: ${name}+${number}"
log "Fetching packages…"
"$FLUTTER" pub get

if [[ "${SIGNED:-0}" == "1" ]]; then
  method="${EXPORT_METHOD:-development}"
  log "Building SIGNED .ipa (export method: $method)…"
  "$FLUTTER" build ipa --release --export-method "$method" --build-name="$name" --build-number="$number"
  src="$(ls -t build/ios/ipa/*.ipa 2>/dev/null | head -1 || true)"
  if [[ -z "$src" ]]; then
    echo "error: no .ipa produced. Is signing configured in Xcode?" >&2
    echo "       Open ios/Runner.xcworkspace → Runner → Signing & Capabilities." >&2
    exit 1
  fi
  dest="$DIST_DIR/preuzmi-${version}.ipa"
  cp "$src" "$dest"
  log "→ $dest ($(du -h "$dest" | cut -f1))"
else
  log "Building UNSIGNED release .app…"
  "$FLUTTER" build ios --release --no-codesign --build-name="$name" --build-number="$number"
  app="build/ios/iphoneos/Runner.app"
  [[ -d "$app" ]] || { echo "error: $app not found." >&2; exit 1; }

  log "Packaging into an .ipa…"
  work="$(mktemp -d)"
  mkdir -p "$work/Payload"
  cp -R "$app" "$work/Payload/Runner.app"
  dest="$DIST_DIR/preuzmi-${version}-unsigned.ipa"
  rm -f "$dest"
  ( cd "$work" && zip -qr9 "$dest" Payload )
  rm -rf "$work"
  log "→ $dest ($(du -h "$dest" | cut -f1))"
  log "Unsigned: install via Sideloadly/AltStore, or run 'SIGNED=1 scripts/build-ipa.sh' after setting up Xcode signing."
fi

log "Done. .ipa in $DIST_DIR"
