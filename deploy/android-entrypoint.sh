#!/bin/sh
set -e
build_type="${ANDROID_BUILD_TYPE:-release}"
case "$build_type" in
  release|debug) ;;
  *) echo "ERROR: unknown ANDROID_BUILD_TYPE=$build_type (expected release|debug)" >&2; exit 1 ;;
esac

cd android
chmod +x gradlew
# Version metadata from the VERSION file (see deploy/start.sh): passed as Gradle
# properties so app/build.gradle can use them with fallback defaults.
gradle_args=""
if [ -n "${ML_VERSION_NAME:-}" ]; then gradle_args="$gradle_args -PversionName=$ML_VERSION_NAME"; fi
if [ -n "${ML_VERSION_CODE:-}" ]; then gradle_args="$gradle_args -PversionCode=$ML_VERSION_CODE"; fi
./gradlew "assemble${build_type}" --no-daemon $gradle_args
mkdir -p /output
# We require a SIGNED apk; the AGP-generated `*-unsigned.apk` is rejected on
# purpose. For `release` there is no debug-key fallback (an unsigned APK would
# lack META-INF/MANIFEST.MF + signature blocks); for `debug` the standard
# AGP debug keystore signs the app-debug.apk.
out_name="app-${build_type}.apk"
apk=$(find app/build/outputs -name "*.apk" ! -name "*-unsigned.apk" -print -quit 2>/dev/null)
if [ -z "$apk" ]; then
  echo "ERROR: no signed ${out_name} produced — only *-unsigned.apk found." >&2
  if [ "$build_type" = "release" ]; then
    echo "  - Is web/android/keystore.properties mounted read-only into /src/android?" >&2
    echo "  - Are KEYSTORE_FILE / KEYSTORE_PASSWORD / KEYSTORE_KEY_ALIAS / KEYSTORE_KEY_PASSWORD correct in deploy/.env?" >&2
  fi
  exit 1
fi
cp "$apk" "/output/${out_name}"
echo "✔ APK written to /output/${out_name} ($(du -h /output/${out_name} | cut -f1))"