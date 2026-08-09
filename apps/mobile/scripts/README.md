# Mobile build scripts

Local production builds for the Flutter app. Artifacts land in `apps/mobile/dist/`
(git-ignored), named with the `pubspec.yaml` version, e.g. `preuzmi-1.0.0_1.apk`.

Run from `apps/mobile`:

## Android (APK)

```bash
scripts/build-apk.sh            # one universal release APK  → dist/preuzmi-<ver>.apk
SPLIT=1 scripts/build-apk.sh    # smaller per-ABI APKs       → dist/preuzmi-<ver>-<abi>.apk
```

## iOS (IPA) — macOS + Xcode only

```bash
scripts/build-ipa.sh            # UNSIGNED  → dist/preuzmi-<ver>-unsigned.ipa
SIGNED=1 scripts/build-ipa.sh   # SIGNED    → dist/preuzmi-<ver>.ipa
```

- **Unsigned** needs no Apple account. Install by sideloading with
  [Sideloadly](https://sideloadly.io) or [AltStore](https://altstore.io), which
  re-sign it with your Apple ID on install.
- **Signed** requires signing set up once in Xcode
  (`open ios/Runner.xcworkspace` → *Runner → Signing & Capabilities* → select
  your Team). Pick the export type with `EXPORT_METHOD` (`development` (default),
  `ad-hoc`, or `app-store`).

## Environment

The scripts auto-detect Flutter (PATH or Homebrew) and, on macOS, the Android
SDK / Android Studio JDK. Override any of them:

```bash
FLUTTER=/path/to/flutter ANDROID_HOME=… JAVA_HOME=… scripts/build-apk.sh
```
