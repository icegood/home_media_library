#!/bin/sh
set -eu

cd "$(dirname "$0")"

if [ "$#" -ne 1 ]; then
  echo "Usage: sh deploy/start.sh [prod|local-build]" >&2
  echo "  prod        Pull and run versioned production images from deploy/compose.yaml." >&2
  echo "  local-build Build local backend/web sources, then run the same stack." >&2
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
    docker compose -f compose.yaml pull
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
    export PROJECT_VERSION API_IMAGE WEB_IMAGE VCS_REF BUILD_DATE ENV_FILE RUNTIME_DIR

    docker buildx bake --allow=fs.read=../backend --allow=fs.read=../web --load -f compose.yaml -f compose.local.yaml
    docker compose --env-file .env -f compose.yaml -f compose.local.yaml up -d --remove-orphans
    ;;

  *)
    echo "Usage: sh deploy/start.sh [prod|local-build]" >&2
    echo "  prod        Pull and run versioned production images from deploy/compose.yaml." >&2
    echo "  local-build Build local backend/web sources, then run the same stack." >&2
    exit 2
    ;;
esac
