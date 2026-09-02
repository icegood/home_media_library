#!/bin/sh
set -eu

cd "$(dirname "$0")"

if [ "$#" -ne 1 ]; then
  echo "Usage: sh deploy/start.sh [prod|local-build|e2e|android]" >&2
  echo "  prod        Pull and run versioned production images from deploy/compose.yaml." >&2
  echo "  local-build Build local backend/web sources, then run the same stack." >&2
  echo "  e2e         Run Playwright end-to-end tests against the running stack." >&2
  echo "  android     Build a signed release APK for Android." >&2
  exit 2
fi

mode="$1"
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-media_library}"

ensure_env() {
  env_path="$1"
  default_path="$2"
  if [ ! -f "$env_path" ]; then
    if [ -f "$default_path" ]; then
      cp "$default_path" "$env_path"
      chmod 600 "$env_path"
      echo "Created $env_path from $default_path. Edit it, then run this script again." >&2
      exit 1
    fi
    echo "Missing $env_path. Copy $default_path to $env_path and edit it first." >&2
    exit 1
  fi
}

require_project_version() {
  source_hint="$1"
  if [ -z "${PROJECT_VERSION:-}" ]; then
    echo "PROJECT_VERSION is required in ${source_hint}." >&2
    exit 1
  fi
  if [ "$PROJECT_VERSION" = "__PROJECT_VERSION__" ]; then
    echo "PROJECT_VERSION placeholder was not replaced in ${source_hint}." >&2
    exit 1
  fi
}

case "$mode" in
  prod | production | published | deploy)
    ensure_env .env .env.default
    set -a
    . ./.env
    set +a
    require_project_version "deploy/.env"
    resolved_version="$PROJECT_VERSION"
    API_IMAGE="ghcr.io/icegood/home-media-library-api:${resolved_version}"
    WEB_IMAGE="ghcr.io/icegood/home-media-library-web:${resolved_version}"
    ENV_FILE="${ENV_FILE:-.env}"
    export API_IMAGE WEB_IMAGE ENV_FILE
    # Pull only the services that will actually run: the gateway is conditional
    # on the "https" compose profile, so its image is only pulled when the
    # profile is enabled.
    pull_targets="api web"
    for profile in $(printf '%s' "${COMPOSE_PROFILES:-}" | tr ',' ' '); do
      if [ "$profile" = "https" ]; then
        pull_targets="$pull_targets gateway"
      fi
    done
    # shellcheck disable=SC2086
    docker compose -f compose.yaml pull $pull_targets
    docker compose -f compose.yaml rm -sf
    docker compose -f compose.yaml up -d --no-build --remove-orphans
    ;;

  local-build | local | build)
    ensure_env .env .env.default
    set -a
    . ./.env
    set +a

    if [ "${RUNTIME_DIR:-}" = "./runtime" ]; then
      RUNTIME_DIR="../runtime"
    fi

    if [ ! -f ../VERSION ]; then
      echo "VERSION is required for local-build." >&2
      exit 1
    fi
    PROJECT_VERSION="$(sed -n '1p' ../VERSION)"
    require_project_version "../VERSION"
    API_IMAGE="media-library-api:${PROJECT_VERSION}"
    WEB_IMAGE="media-library-web:${PROJECT_VERSION}"
    VCS_REF="${VCS_REF:-$(git -C .. rev-parse --short HEAD 2>/dev/null || printf local)}"
    if [ -z "${BUILD_DATE:-}" ] && [ "$VCS_REF" != "local" ]; then
      BUILD_DATE="$(git -C .. show -s --format=%cI "$VCS_REF" 2>/dev/null || printf unknown)"
    fi
    BUILD_DATE="${BUILD_DATE:-unknown}"
    ENV_FILE="${ENV_FILE:-.env}"
    WEB_ASSETS_IMAGE="${WEB_ASSETS_IMAGE:-media-library-web-assets:${PROJECT_VERSION}}"
    export PROJECT_VERSION API_IMAGE WEB_IMAGE VCS_REF BUILD_DATE ENV_FILE RUNTIME_DIR WEB_ASSETS_IMAGE

    # The web/build base image is a shared root reused by the nginx web image
    # (web/Dockerfile) and the Android build (Dockerfile.android). Building it
    # first means both can reuse these heavy layers instead of re-installing
    # node_modules.
    docker build \
      --build-arg VERSION="$PROJECT_VERSION" \
      --build-arg VCS_REF="$VCS_REF" \
      --build-arg BUILD_DATE="$BUILD_DATE" \
      -f Dockerfile.web-assets -t "$WEB_ASSETS_IMAGE" ../web

    # API is built via bake (its FROM refs are registry images: golang, alpine).
    # WEB is built via plain `docker build` because its `build` stage is
    # FROM ${WEB_ASSETS_IMAGE} — a local daemon image produced above; bake
    # tries to resolve that ref from the registry, so we keep web out of
    # bake and let the plain docker driver use the daemon image store.
    docker buildx bake --allow=fs.read=../backend --load -f compose.yaml -f compose.local.yaml api
    docker build \
      --build-arg VERSION="$PROJECT_VERSION" \
      --build-arg VCS_REF="$VCS_REF" \
      --build-arg BUILD_DATE="$BUILD_DATE" \
      --build-arg WEB_ASSETS_IMAGE="$WEB_ASSETS_IMAGE" \
      -f ../web/Dockerfile -t "$WEB_IMAGE" ../web
    docker compose --env-file .env -f compose.yaml -f compose.local.yaml up -d --force-recreate --remove-orphans
    ;;

  e2e)
    ensure_env .env .env.default
    set -a
    . ./.env
    set +a
    base_url="${E2E_BASE_URL:-http://localhost:${WEB_PORT:-8080}}"
    pw_version="$(grep -A1 '"node_modules/@playwright/test"' ../web/package-lock.json | sed -n 's/.*"version": "\([^"]*\)".*/\1/p' | head -1)"
    if [ -z "$pw_version" ]; then
      echo "Cannot resolve the @playwright/test version from web/package-lock.json." >&2
      exit 1
    fi
    echo "Building Playwright runner (@playwright/test ${pw_version})..."
    docker build --build-arg PW_VERSION="$pw_version" \
      -f ../web/Dockerfile.playwright -t media-library-playwright:local ..
    mkdir -p ../web/test-output
    # The stack must already be running (start.sh local-build / prod).
    docker run --rm --network host \
      -e BASE_URL="$base_url" \
      -v "$(cd .. && pwd)/web/test-output:/app/test-output" \
      media-library-playwright:local
    ;;

  android)
    ensure_env .env .env.default
    set -a
    . ./.env
    set +a
    if [ ! -f ../VERSION ]; then
      echo "VERSION is required for android." >&2
      exit 1
    fi
    PROJECT_VERSION="$(sed -n '1p' ../VERSION)"
    require_project_version "../VERSION"
    VCS_REF="${VCS_REF:-$(git -C .. rev-parse --short HEAD 2>/dev/null || printf local)}"
    if [ -z "${BUILD_DATE:-}" ] && [ "$VCS_REF" != "local" ]; then
      BUILD_DATE="$(git -C .. show -s --format=%cI "$VCS_REF" 2>/dev/null || printf unknown)"
    fi
    BUILD_DATE="${BUILD_DATE:-unknown}"
    WEB_ASSETS_IMAGE="${WEB_ASSETS_IMAGE:-media-library-web-assets:${PROJECT_VERSION}}"
    # The android builder user only needs a deterministic, non-root identity
    # inside the build container; it does NOT have to match the host user
    # (that's what MEDIA_UID / MEDIA_GID are for, used by the api service).
    # Use the host uid/gid so build artifacts are owned by the caller; falling
    # back to 1001 avoids any collision with base-image system groups
    # ("users: gid 100" in eclipse-temurin:21-jdk-jammy).
    media_uid="${MEDIA_UID_OVERRIDE:-$(id -u)}"
    media_gid="${MEDIA_GID_OVERRIDE:-$(id -g)}"
    apk_out="$(pwd)/../build/android"
    # --- Release signing ----------------------------------------------------
    # There is NO debug-key fallback: the APK must carry the JKS listed by
    # KEYSTORE_FILE. Without real credentials, build aborts before producing
    # any APK, so a failed run cannot silently ship an unsigned or
    # mis-signed artefact.
    keystore_file="${KEYSTORE_FILE:-}"
    keystore_password="${KEYSTORE_PASSWORD:-}"
    keystore_alias="${KEYSTORE_KEY_ALIAS:-}"
    keystore_key_password="${KEYSTORE_KEY_PASSWORD:-}"
    missing=0
    for kv in "KEYSTORE_FILE:$keystore_file" "KEYSTORE_PASSWORD:$keystore_password" "KEYSTORE_KEY_ALIAS:$keystore_alias" "KEYSTORE_KEY_PASSWORD:$keystore_key_password"; do
      k="${kv%%:*}"
      v="${kv#*:}"
      if [ -z "$v" ] || printf '%s' "$v" | grep -q CHANGE_ME; then
        echo "$k is missing or still has the CHANGE_ME placeholder in deploy/.env." >&2
        missing=1
      fi
    done
    if [ "$missing" -ne 0 ]; then
      echo "Edit deploy/.env and replace the four KEYSTORE_* placeholders." >&2
      exit 1
    fi
    case "$keystore_file" in
      /*) ;;
      *) echo "KEYSTORE_FILE must be an absolute path (got: $keystore_file)." >&2; exit 1 ;;
    esac
    if [ ! -f "$keystore_file" ]; then
      echo "KEYSTORE_FILE does not exist on the host: $keystore_file" >&2
      exit 1
    fi
    keystore_dir="$(dirname "$keystore_file")"
    keystore_name="$(basename "$keystore_file")"
    # Render web/android/keystore.properties for the gradle build; gradle reads
    # it from android/keystore.properties (rootProject.file).
    repo_root="$(cd .. && pwd)"
    keystore_props="${repo_root}/web/android/keystore.properties"
    mkdir -p "$(dirname "$keystore_props")"
    cat > "$keystore_props" <<EOF
storeFile=/keystrokes/${keystore_name}
storePassword=${keystore_password}
keyAlias=${keystore_alias}
keyPassword=${keystore_key_password}
EOF
    chmod 600 "$keystore_props"
    # ---- Build ------------------------------------------------------------
    mkdir -p "$apk_out"
    echo "Building signed Android release APK (uid=${media_uid} gid=${media_gid})..."
    # The web-assets base image is the shared build root also produced by
    # `local-build`. Building it first (cheap when cached) lets the Android
    # image reuse the npm ci / vite build layers instead of reinstalling all
    # dependencies.
    docker build \
      --build-arg VERSION="$PROJECT_VERSION" \
      --build-arg VCS_REF="$VCS_REF" \
      --build-arg BUILD_DATE="$BUILD_DATE" \
      -f Dockerfile.web-assets -t "$WEB_ASSETS_IMAGE" ../web
    docker build \
      --build-arg MEDIA_UID="$media_uid" \
      --build-arg MEDIA_GID="$media_gid" \
      --build-arg WEB_ASSETS_IMAGE="$WEB_ASSETS_IMAGE" \
      -f Dockerfile.android -t media-library-android:local ..
    docker run --rm \
      -v "$keystore_dir:/keystrokes:ro" \
      -v "$keystore_props:/src/android/keystore.properties:ro" \
      -v "$apk_out:/output" \
      media-library-android:local
    echo ""
    echo "APKs:"
    ls -lh "$apk_out"/*.apk 2>/dev/null || echo "  (none found)"
    ;;

  *)
    echo "Usage: sh deploy/start.sh [prod|local-build|e2e|android]" >&2
    echo "  prod        Pull and run versioned production images from deploy/compose.yaml." >&2
    echo "  local-build Build local backend/web sources, then run the same stack." >&2
    echo "  e2e         Run Playwright end-to-end tests against the running stack." >&2
    exit 2
    ;;
esac
