# syntax=docker/dockerfile:1
# The heavy web build (dependencies, lint, tests, dist) lives in the shared
# deploy/Dockerfile.web-assets base image (built as media-library-web-assets:
# VERSION by deploy/start.sh and referenced below). This file only wraps that
# dist into an nginx image; source paths are adapted here, after reuse.
ARG NODE_IMAGE=node:22-alpine
# Shared build root produced by deploy/start.sh (local-build/android) or pushed
# by publish-images.yml. The default keeps plain `docker build` from emitting
# InvalidDefaultArgInForm (FROM with an empty arg is not a valid build).
ARG WEB_ASSETS_IMAGE=media-library-web-assets:local

FROM ${WEB_ASSETS_IMAGE} AS build

FROM ${NODE_IMAGE} AS dev
RUN apk add --no-cache git bash

FROM nginx:1.27-alpine
ARG VERSION
ARG VCS_REF
ARG BUILD_DATE
ARG SOURCE_URL=https://github.com/icegood/home_media_library
LABEL org.opencontainers.image.title="Media Library Web" \
      org.opencontainers.image.description="React web UI for the self-hosted media library" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="${SOURCE_URL}"
# nginx's image entrypoint envsubst-renders this template at container start,
# binding the listen port to WEB_INTERNAL_PORT (default 8080).
ENV WEB_INTERNAL_PORT=8080
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 8080