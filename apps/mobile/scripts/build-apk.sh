#!/usr/bin/env bash
#
# Build a production (release) Android APK for the preuzmi.me mobile app and copy
# it into apps/mobile/dist/.
#
# Usage:
#   scripts/build-apk.sh            # one universal release APK
#   SPLIT=1 scripts/build-apk.sh    # per-ABI APKs (smaller downloads)
#
# Env overrides: FLUTTER, ANDROID_HOME, JAVA_HOME.

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/_common.sh"

cd "$MOBILE_DIR"
mkdir -p "$DIST_DIR"
version="$(version_tag)"
name="$(build_name)"
number="$(build_number)"

log "Flutter: $FLUTTER"
log "Version: ${name}+${number}"
log "Java:    ${JAVA_HOME:-<system>}"
log "Android: ${ANDROID_HOME:-<system>}"

log "Fetching packages…"
"$FLUTTER" pub get

if [[ "${SPLIT:-0}" == "1" ]]; then
  log "Building split-per-ABI release APKs…"
  "$FLUTTER" build apk --release --split-per-abi --build-name="$name" --build-number="$number"
  for abi in armeabi-v7a arm64-v8a x86_64; do
    src="build/app/outputs/flutter-apk/app-${abi}-release.apk"
    [[ -f "$src" ]] || continue
    dest="$DIST_DIR/preuzmi-${version}-${abi}.apk"
    cp "$src" "$dest"
    log "→ $dest ($(du -h "$dest" | cut -f1))"
  done
else
  log "Building universal release APK…"
  "$FLUTTER" build apk --release --build-name="$name" --build-number="$number"
  src="build/app/outputs/flutter-apk/app-release.apk"
  dest="$DIST_DIR/preuzmi-${version}.apk"
  cp "$src" "$dest"
  log "→ $dest ($(du -h "$dest" | cut -f1))"
fi

log "Done. APK(s) in $DIST_DIR"
