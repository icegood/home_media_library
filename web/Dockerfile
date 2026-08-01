FROM --platform=$BUILDPLATFORM node:22-alpine AS build
ARG VERSION
WORKDIR /src

COPY package.json package-lock.json* ./
RUN npm ci

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
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 8080
