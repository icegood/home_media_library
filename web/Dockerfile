# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:22-alpine AS build
ARG VERSION
ARG VCS_REF
ARG BUILD_DATE
ENV APP_VERSION=$VERSION
ENV VCS_REF=$VCS_REF
ENV BUILD_DATE=$BUILD_DATE
WORKDIR /src

COPY package.json package-lock.json* ./
# Keep npm's tarball cache in a BuildKit cache mount so `npm ci` reuses already
# downloaded packages when the dependency set changes.
RUN --mount=type=cache,target=/root/.npm \
    npm ci

COPY . .
RUN npm test && npm run build

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
