---
name: add-feature
description: "Use when implementing or changing any user-visible behaviour in the media library (UI, API, scanner, map, playback). End-to-end checklist from requirements to deploy."
---

# Add / change a feature

1. **Locate rules.** Read root `CLAUDE.md` and the `CLAUDE.md` of every area you
   touch (backend package, `web/`, `docs/`).
2. **Requirements first.** Edit `docs/requirements.md` in the matching section:
   describe the required behaviour (not a changelog entry).
3. **Backend (if needed).**
   - Schema → `add-db-migration` skill.
   - New/changed route → `add-api-endpoint` skill.
   - Store methods in **both** `sqlite.go` and `postgres.go`.
   - Background work → job handle + `jobpool`, never the interactive handle.
   - Tests in the package's `*_test.go`.
4. **Frontend (if needed).**
   - REST calls only through `web/src/api.ts`; types in `web/src/types.ts`.
   - Visible text → `add-ui-text` skill (14 translations).
   - Errors via `ErrorPopup`, confirms via `translateMessage`; no `alert()`.
   - Test in `web/src/App.test.tsx` (or the focused `*.test.ts`).
5. **Build + test + deploy locally:** `sh deploy/start.sh local-build`
   (`update-ui-after-build` skill). Fix failures; do not skip tests.
6. **Browser/e2e only if asked** (`e2e-run` skill).
7. **Report** in short form (`short-answers`). Commit only when asked, with the
   message format `1) <behaviour change> 2) <next>` and no `VERSION` bump.
