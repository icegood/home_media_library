#!/bin/sh
set -e
cd android
chmod +x gradlew
./gradlew assembleRelease --no-daemon
mkdir -p /output
apk=$(find app/build/outputs -name "*.apk" -print -quit 2>/dev/null)
if [ -z "$apk" ]; then
  echo "ERROR: no APK produced" >&2
  exit 1
fi
cp "$apk" /output/app-release.apk
echo "✔ APK written to /output/app-release.apk ($(du -h /output/app-release.apk | cut -f1))"
