FROM golang:1.23-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go mod tidy \
    && gofmt -w . \
    && go test ./... \
    && CGO_ENABLED=0 go build -o /media-api ./cmd/server

FROM alpine:3.21
ARG UID=1000
ARG GID=1000
ARG DOCKER_GID=998
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
ARG SOURCE_URL=https://github.com/icegood/home_media_library
LABEL org.opencontainers.image.title="Media Library API" \
      org.opencontainers.image.description="Go backend for the self-hosted media library" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="${SOURCE_URL}"
RUN apk add --no-cache exiftool ffmpeg docker-cli su-exec \
    && addgroup -g ${GID} app \
    && addgroup -g ${DOCKER_GID} dockersock \
    && adduser -D -u ${UID} -G app app \
    && addgroup app dockersock \
    && mkdir -p /data /gateway \
    && chown -R ${UID}:${GID} /data /gateway
COPY --from=build /media-api /usr/local/bin/media-api
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["media-api"]
