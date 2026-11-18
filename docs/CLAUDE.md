# Docs (`docs/`)

- `requirements.md` — **single source of truth** for functional behaviour. Update
  it in the same change as any user-visible behaviour; write the rule as the
  required behaviour (not a changelog), in the matching section.
- `openapi.yaml` — identical copy of `backend/internal/api/openapi.yaml`; keep
  both in sync.
- `architecture.md` — domain, security boundaries, DB choice, scanner pipeline,
  roadmap. Update when an architectural fact changes.
- `https-domain-setup.md` — operator guide for the Caddy/HTTPS gateway.
