---
name: ship-release
description: "Use when the user says 'commit and deploy', 'ship it', 'as usual', or asks to publish media library images and deploy to the NAS / production."
---

# Commit, publish images, deploy to NAS

Only on explicit request. **No `VERSION` bump** unless asked.

1. **Commit** (on `main`) only the intended files; message:
   `1) <behaviour change> 2) <next>` + the attribution trailer.
2. **Push:** `git push icegood main`.
3. **Publish images:** dispatch `.github/workflows/publish-images.yml` on `main`
   via the GitHub REST API (no `gh`). Token: `git credential fill` for
   `github.com` (or `~/.git-credentials` / `.github-token`). Repo
   `icegood/home_media_library`.
   ```sh
   curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
     -H "Accept: application/vnd.github+json" \
     https://api.github.com/repos/icegood/home_media_library/actions/workflows/publish-images.yml/dispatches \
     -d '{"ref":"main"}'
   ```
4. **Wait** for the run whose `head_sha` equals the pushed SHA to finish with
   `conclusion: success` (poll `.../actions/runs?branch=main&event=workflow_dispatch`).
5. **Deploy on NAS** (`home-server` skill, host 192.168.13.1):
   `cd /mnt/sdcard/scripts/media-library-install && sh ./start.sh prod`.
6. **Reload nginx_runner** (container IP changes → 502 otherwise):
   `docker exec nginx_runner nginx -s reload`.
7. **Verify:** `curl -s -o /dev/null -w '%{http_code}' http://192.168.13.1:8099/api/v1/setup`
   → `200`. A 502 right after restart may be the api still starting — recheck
   before reporting.

Chain steps 4–7 in one background job so no time is lost. Report in one line.
