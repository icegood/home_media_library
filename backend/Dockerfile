# syntax=docker/dockerfile:1
ARG GOLANG_IMAGE=golang:1.23-alpine
FROM --platform=$BUILDPLATFORM ${GOLANG_IMAGE} AS build
ARG VERSION
ARG VCS_REF
ARG BUILD_DATE
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
# Download modules in their own layer (reused unless go.mod/go.sum change) and
# keep both module and build caches in BuildKit cache mounts so repeat
# `deploy/start.sh local-build` runs recompile only what actually changed.
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod tidy \
    && gofmt -w . \
    && go test ./... \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
       -ldflags "-X main.version=${VERSION} -X main.revision=${VCS_REF} -X main.buildDate=${BUILD_DATE}" \
       -o /media-api ./cmd/server

# Dev toolchain stage: same base image as `build`, so the Dev Containers
# configuration (which builds this target) always matches the build version.
FROM ${GOLANG_IMAGE} AS dev
RUN apk add --no-cache git bash curl exiftool ffmpeg \
    && go install golang.org/x/tools/gopls@v0.17.1

FROM alpine:3.21
ARG VERSION
ARG VCS_REF
ARG BUILD_DATE
ARG SOURCE_URL=https://github.com/icegood/home_media_library
LABEL org.opencontainers.image.title="Media Library API" \
      org.opencontainers.image.description="Go backend for the self-hosted media library" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="${SOURCE_URL}"
RUN apk add --no-cache exiftool ffmpeg docker-cli su-exec \
    && mkdir -p /data
COPY --from=build /media-api /usr/local/bin/media-api
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["media-api"]
