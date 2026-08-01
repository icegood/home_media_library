# Media Library

Self-hosted, multi-user photo and video library. Administrators map arbitrary
folders from disk into library items and grant each user read access. The web
application is responsive and can also be packaged as an Android application
with Capacitor.

## Why

For a long time ago i've been trying to get along with solutions that already exist. And tried them to apply to my set of usecases. Namely, one or many media repositories with updating stuff outside of application and different users that obtain different (read/only) rights for some parts of these repositories. Emby and [jellyfin]([https://](https://github.com/jellyfin/jellyfin)https:/) were ones that more or less satisfy this but with bugs, especially during refresh. It wasn't acceptable. Other multiuser solution, [immich](https://github.com/immich-app/immich) went other way: total separation between users. Therefore, no library administrator needed. Everyone does everything with own part. It has own pros, but this is not my way (it would imply dublication of common pictures, at least). One of

descent forks adresses it by adding concept of shared data

([open-noodle](https://github.com/open-noodle/gallery)) but since they want be compatible, i didnt want dig down inside it much. And one more pont was a **the last straw** :[jellyfin hangs](https://cybernews.com/tech/jellyfin-founders-step-down-future-uncertain/). So, time has come...


## More about this

This section is about solution from my words (all other sections but previous one from here and >99% of files belong to codex, and if you think i read them you are soo wroong :)). Since i'm not good in web applications at all, it is not under my skills, i relied to AI. Let it flows... All a tried hard is to get primary use cases work for me  and don't let db to get unnecessary data. Some db normalization, some async optimization, some obvious UI convenience. That's it.

If you think it could be done better - no problem, PR to me or fork. 

Why it is done like this right now:

* regarding folder-like views as primary one. It is up to user to organize data on disk before feeding it to media libraries. User knows (or at least should know) more than general purpose solution. For all others [plex](https://watch.plex.tv/) is your way to go. Immich's timeline would be fine as well once you added all dates for all of your files (which immich lacks to add). And if you have thousands since that time, when no timestapm present here (since exif was absent, or no relevant exif at all) then it might be a challenge...
* I like maps from immich so much. I missed it in emby... But again, so many old images dont have gps as well... Added here as filed to db (with date above)
* User would have own 'space' i.e. he/she can create own favorites over data as he/she wishes.
* All modifications above don't touch original data. They simply mustn't. It is an axiom.
* Video playing. Of course, you should avoid reencoding as much as you can while playing video. Therefore, lets user shafre information first what codec's browser supports. And if we fail then lets fallback to predefined one... Probably it should be user specific, since each user might have own codecs to support. But for now let it be common one and we will see in future.

## Architecture

- `backend/` — Go REST API, folder scanner, authorization, SQLite/PostgreSQL
- `web/` — React + TypeScript browser UI and Capacitor Android shell
- `deploy/` — container configuration and sample environment

Media files remain in their original folders. The database stores users,
libraries, permissions, indexed file paths, extracted metadata, thumbnails and
editable coordinates. A library's relative directory hierarchy is exposed
unchanged by the API.

## Quick start

Copy the configuration and replace the secrets first:

```sh
cp deploy/.env.default .env
```

Start the system:

```sh
sh deploy/start.sh prod
```

This pulls the versioned production images from `.env` / `deploy/.env.default`.
To build local source instead, run:

```sh
sh deploy/start.sh local-build
```

HTTP is enabled by default and HTTPS is disabled. After creating the initial
administrator, open **Settings → Network access** to enable or disable either
protocol. At least one must remain enabled. Changes are persisted and Caddy
reloads them automatically without Docker access or a container restart.

When enabling HTTPS, enter the public DNS name and Let's Encrypt contact email
in the same administrator screen.

Caddy obtains the certificate directly from Let's Encrypt, renews it
automatically, and keeps ACME state in external host folders. Before starting HTTPS:

- `PUBLIC_DNS` must be a public DNS name whose A/AAAA record points to this host.
- Public inbound TCP ports 80 and 443 must reach this host.
- Public inbound UDP 443 is optional and enables HTTP/3.
- Let's Encrypt does not issue normal public certificates for `localhost` or
  private-only names.

In HTTPS-only mode no HTTP application listener or HTTP-to-HTTPS redirect is
configured. Let’s Encrypt may still use port 80 temporarily for the ACME HTTP
challenge; TLS-ALPN validation on port 443 is also supported by Caddy.

On the first startup, the browser presents a setup form for the initial
administrator login and password. The setup endpoint is disabled permanently
as soon as the first user is created. The bootstrap user state is stored in the
external runtime folder, so container recreation does not reopen setup.

Runtime state is intentionally mounted through one host bind, not Docker named
volumes:

- `${RUNTIME_DIR:-./runtime}` -> `/runtime`
  - `/runtime/app-data` — SQLite database, users, settings
  - `/runtime/app-config` — generated gateway config
  - `/runtime/caddy-data` — Caddy/Let’s Encrypt data
  - `/runtime/caddy-config` — Caddy runtime config
  - `/runtime/thumbnails` — generated media thumbnails

Recreating containers does not remove this folder. Back it up like normal
application data.

Optional Emby import does not create a default runtime folder. If you need it,
temporarily mount the folder that contains Emby's `data/library.db` and
`data/users.db` into the API container, then open **Settings → Library → Emby
import** and enter the container paths. The importer copies:

- Emby libraries (`MediaItems.Type=4` under `Media Folders`)
- Emby library roots (`ItemExtradata.Value.PathInfos` for `LibraryOptions`)
- Emby users (`LocalUsersv2`, login and password)
- library-user read links (`UserItemShares`, `ShareLevel >= 100`)

Emby stores local passwords as unsalted SHA-1 hashes. The importer keeps those
legacy hashes only to let users sign in with their existing Emby password. On
the first successful login the password hash is upgraded to bcrypt and persisted.
If an Emby user has no importable local password, that user receives a temporary
generated password shown once in the import result. Add path mappings only when
Emby and this container see the same media folders under different mount
prefixes.

## Deploy and run

### Prerequisites

- Docker with Compose v2 (for the containerized setup).
- A folder with media files to serve (point `MEDIA_ROOT` at it).
- A `.env` file with `JWT_SECRET` set to 32+ random characters.

### Production deployment from release artifact

Every publish workflow creates a deployment artifact:

```text
media-library-deploy-<version>.tar.gz
```

Download it from:

```text
GitHub → Actions → Publish Docker images → workflow run → Artifacts
```

Unpack it on your Docker host:

```sh
mkdir -p media-library
tar -xzf media-library-deploy-<version>.tar.gz -C media-library --strip-components=1
cd media-library
```

The unpacked folder contains only deployment files:

```text
compose.yaml
.env.default
start.sh
```

Create your real environment file:

```sh
cp .env.default .env
chmod 600 .env
```

Edit `.env`:

- `JWT_SECRET` — replace with a real 32+ character secret.
- `MEDIA_ROOT` — host folder with your media files.
- `MEDIA_UID` / `MEDIA_GID` — host user/group id that should own runtime files.
- `DOCKER_GID` — host docker socket group id, if you want the admin stop button.
- `HTTP_PORT` / `HTTPS_PORT` — host ports to expose.
- `RUNTIME_DIR` — host folder for database, config, certificates and thumbnails.

Useful commands:

```sh
id -u
id -g
getent group docker | cut -d: -f3
```

If GHCR packages are private, login before starting:

```sh
echo '<github-token-with-read-packages>' | docker login ghcr.io -u '<github-user>' --password-stdin
```

Start:

```sh
sh start.sh prod
```

Open:

```text
http://<host>:<HTTP_PORT>
```

The first browser visit creates the initial administrator login/password. Runtime
data is kept under `RUNTIME_DIR`, so recreating containers does not delete the
database, thumbnails, generated gateway config or Let's Encrypt state.

To update later, unpack a newer artifact over the deployment folder while
preserving `.env` and `RUNTIME_DIR`, then run:

```sh
sh start.sh prod
```

### Local source build with Compose

```sh
cp deploy/.env.default .env
```

Edit `.env`: set `JWT_SECRET`, `MEDIA_ROOT` (the host folder with your media),
and `MEDIA_UID`/`MEDIA_GID` to your host user and group IDs — the API container
runs as that user (the access boundary), so it can read the mounted media and
write the host-mounted `RUNTIME_DIR`. Set `DOCKER_GID` to the host docker
socket group id so the admin UI **Stop Docker container** action can call Docker
and stop the API container for real:

```sh
getent group docker | cut -d: -f3
```

All api container settings are read from `.env` (via `env_file`); libraries
reference explicit absolute paths. Then build and run from source:

```sh
sh deploy/start.sh local-build
```

Open `http://localhost:${HTTP_PORT:-8080}` and complete the first-run
administrator setup. After changing source, use the same starter so old containers
cannot keep serving stale API or UI code. It removes only Compose containers;
bind-mounted runtime data, database, thumbnails, Caddy/Let’s Encrypt state, and
media folders are preserved.

The API container mounts `/var/run/docker.sock` only so the administrator
**Stop Docker container** button can run `docker stop` on its own container.
The adjacent **Stop server process** button sends an in-process shutdown signal;
that is useful for container-less deployments, but in Docker it looks like a
process exit and `restart: unless-stopped` can start the API again.

The local-build starter derives `API_IMAGE`, `WEB_IMAGE`, `ENV_FILE` and runtime
paths from `.env`, then runs Compose with the production file plus the local
build overlay:

```sh
cd deploy
docker compose --env-file ../.env -f compose.yaml -f ../compose.local.yaml build
docker compose --env-file ../.env -f compose.yaml -f ../compose.local.yaml rm -sf
docker compose --env-file ../.env -f compose.yaml -f ../compose.local.yaml up -d --remove-orphans
```

Prefer `sh deploy/start.sh local-build`; raw Compose intentionally fails unless
the derived image variables are supplied.

`deploy/start.sh local-build` reads the project version only from
[`VERSION`](VERSION) and passes it into Docker image tags and image labels.

### Project and Docker image versioning

Version source is mode-specific:

- published/prod: `PROJECT_VERSION` from `deploy/.env` / release `.env`
- local-build: [`VERSION`](VERSION) from the source checkout

Published Compose uses:

- `ghcr.io/icegood/home-media-library-api:<version>`
- `ghcr.io/icegood/home-media-library-web:<version>`

Both produced images also include OCI metadata labels for version, git revision
and build timestamp. Do not put image names in `.env`; `deploy/start.sh` derives
them from the selected mode's single version source:

- published: `ghcr.io/icegood/home-media-library-{api,web}:<PROJECT_VERSION from .env>`
- local-build: `media-library-{api,web}:<VERSION file>`

To run already-published images from a full source checkout instead of rebuilding:

```sh
sh deploy/start.sh prod
```

To build local Docker images from source instead:

```sh
sh deploy/start.sh local-build
```

For normal production deployment, prefer the release artifact described above.
It contains only:

- `deploy/compose.yaml`
- `deploy/.env.default`
- `deploy/start.sh`

[`deploy/compose.yaml`](deploy/compose.yaml) is production-only and contains
versioned images, not local build contexts. Local source builds add
[`compose.local.yaml`](compose.local.yaml) through `sh deploy/start.sh local-build`.
Published deployments use only the files under `deploy/`, so the server does
not need `backend/` or `web/`.

### Publishing Docker images

Publishing is handled by GitHub Actions. The workflow lives at
[`.github/workflows/publish-images.yml`](.github/workflows/publish-images.yml)
and publishes versioned `api` and `web` images to GHCR:

```text
ghcr.io/<owner>/home-media-library-api:<version>
ghcr.io/<owner>/home-media-library-web:<version>
```

By default the workflow publishes a multi-architecture manifest for:

```text
linux/amd64
linux/arm64
```

Run it in GitHub from **Actions → Publish Docker images → Run workflow**.

To run the same workflow locally, use [`act`](https://github.com/nektos/act).
Create a local token file; do not commit it:

```sh
printf '%s\n' '<github-token-with-write-packages>' > .github-token
chmod 600 .github-token
```

Then run:

```sh
sh scripts/run-publish-action-local.sh
```

Useful overrides:

```sh
VERSION=0.2.0 PUSH_LATEST=true GITHUB_ACTOR=icegood IMAGE_NAMESPACE=icegood/home-media-library PLATFORMS=linux/amd64,linux/arm64 sh scripts/run-publish-action-local.sh
```

The token needs permission to write packages. For GHCR this normally means a
classic PAT with `write:packages` (and `read:packages`; `repo` if publishing
private package images tied to a private repo).

### Building the images individually

Compilation and tests run inside the build containers, so no Go or Node
toolchain is needed on the host. Both builds fail on test errors:

```sh
docker build -t media-library-api ./backend   # go test ./... then compile
docker build -t media-library-web ./web       # npm test && npm run build
```

For the backend image, pass your host UID/GID so the bind-mounted runtime stays
writable (the same values as `MEDIA_UID`/`MEDIA_GID` above):

```sh
docker build --build-arg UID=$(id -u) --build-arg GID=$(id -g) -t media-library-api ./backend
```

## API documentation

The complete HTTP API is documented as an OpenAPI 3.0 spec:

- **Interactive Swagger UI** — served by the API at `/swagger`
  (`http://localhost:8080/swagger` directly, or your host's HTTP/HTTPS port
  through the gateway).
- **Spec file** — [`docs/openapi.yaml`](docs/openapi.yaml), the canonical
  document embedded in the backend binary.

See [`docs/architecture.md`](docs/architecture.md) for boundaries and the
production roadmap.

## Video playback

The browser advertises its H.264, H.265 and VP9 support to the playback
endpoint. Matching source video is served unchanged with byte-range seeking.
Otherwise FFmpeg transcodes as a fragmented stream to the fallback codec chosen
in **Admin → Settings**. Its fixed accepted values are `h264`, `h265`, and
`vp9`. H.264/H.265 use AAC audio in MP4; VP9 uses Opus audio in WebM. `h264` is
the initial and safest default.
