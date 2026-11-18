# Android shell (Capacitor 7)

- Build only with `sh deploy/start.sh android` (signed release APK).
- The app version is `ANDROID_VERSION` (release tag `android-v<version>`). Bump
  it **only** when the native shell changes (`web/android/**`,
  `web/capacitor.config.ts`) — never for server/UI releases (`VERSION`).
- `keystore.properties` holds signing secrets: never commit (use
  `keystore.properties.example`). `.gradle/` and `app/build/` are output.
- The shell loads the SPA; UI changes belong in `web/src`, not here.
