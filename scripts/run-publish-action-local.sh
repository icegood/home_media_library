#!/bin/sh
set -eu

TOKEN_FILE="${TOKEN_FILE:-.github-token}"
VERSION="${VERSION:-$(sed -n '1p' VERSION 2>/dev/null || printf 0.1.0)}"
PUSH_LATEST="${PUSH_LATEST:-true}"
GITHUB_ACTOR="${GITHUB_ACTOR:-icegood}"
IMAGE_NAMESPACE="${IMAGE_NAMESPACE:-icegood/home-media-library}"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
ACT_RUNNER_IMAGE="${ACT_RUNNER_IMAGE:-catthehacker/ubuntu:act-latest}"

if ! command -v act >/dev/null 2>&1; then
  echo "act is required to run GitHub Actions locally: https://github.com/nektos/act" >&2
  exit 1
fi

if [ ! -s "$TOKEN_FILE" ]; then
  echo "Create $TOKEN_FILE with a GitHub token that can write packages." >&2
  echo "The file is gitignored and is passed to act as GITHUB_TOKEN." >&2
  exit 1
fi

act workflow_dispatch \
  --workflows .github/workflows/publish-images.yml \
  --job publish \
  --platform ubuntu-latest="$ACT_RUNNER_IMAGE" \
  --actor "$GITHUB_ACTOR" \
  --secret-file /dev/null \
  --secret GITHUB_TOKEN="$(sed -n '1p' "$TOKEN_FILE")" \
  --input version="$VERSION" \
  --input push_latest="$PUSH_LATEST" \
  --input image_namespace="$IMAGE_NAMESPACE" \
  --input platforms="$PLATFORMS"
