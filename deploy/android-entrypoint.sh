#!/bin/sh
set -e
cd android
chmod +x gradlew
./gradlew assembleRelease --no-daemon
mkdir -p /output
# We require a SIGNED app-release.apk; the AGP-generated
# `app-release-unsigned.apk` is rejected on purpose — there is no debug-key
# fallback, and an unsigned APK would lack META-INF/MANIFEST.MF + signature
# blocks.
apk=$(find app/build/outputs -name "*.apk" ! -name "*-unsigned.apk" -print -quit 2>/dev/null)
if [ -z "$apk" ]; then
  echo "ERROR: no signed app-release.apk produced — only *-unsigned.apk found." >&2
  echo "  - Is web/android/keystore.properties mounted read-only into /src/android?" >&2
  echo "  - Are KEYSTORE_FILE / KEYSTORE_PASSWORD / KEYSTORE_KEY_ALIAS / KEYSTORE_KEY_PASSWORD correct in deploy/.env?" >&2
  exit 1
fi
cp "$apk" /output/app-release.apk
echo "✔ APK written to /output/app-release.apk ($(du -h /output/app-release.apk | cut -f1))"
