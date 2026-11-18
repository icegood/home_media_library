# AGENTS.md

Project rules live in `CLAUDE.md` files, organised hierarchically by feature.
Start with the root `CLAUDE.md` (project-wide hard rules and an index of
per-area rules files), then read the `CLAUDE.md` nearest to the code you edit
(e.g. `backend/internal/store/CLAUDE.md`, `web/CLAUDE.md`).

Skills live in `.claude/skills/` (`.github/skills` is a symlink to it).

Non-negotiable summary:
- Build/test/deploy only via `sh deploy/start.sh local-build|prod|e2e|android`;
  never raw `docker build`, `docker compose up --build`, `npm test`, `npm run build`.
- SQLite and Postgres stores and migrations change together.
- `docs/requirements.md` is updated with every user-visible change.
- No `VERSION` bump unless asked.
