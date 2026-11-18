---
name: add-ui-text
description: "Use whenever media library web UI gets new or changed visible text: labels, tooltips, aria-labels, help text, confirm dialogs, status lines, server problem() messages, or '%s' templates. Adds all 14 translations."
---

# Add UI text + translations

English is canonical in source (tests assert it). Translation happens at render
time (`web/src/i18n.ts` rewrites DOM text + attributes). Every new string needs
a row in **all 14 languages**, in this exact order:

`ua, de, nl, fi, sv, pl, che, slo, hu, es, it, sl, no, pt`

(`che` = Czech, `slo` = Slovak, `sl` = Slovenian, `ua` = Ukrainian.)

## Where

| Kind of text | File |
|---|---|
| Labels, buttons, tooltips, aria-labels, help, status lines, UI `%s` templates | `web/src/i18n-ui.ts` (`UI_ROWS`) |
| Validation, error popups, backend `problem()` texts, message `%s` templates | `web/src/i18n-messages.ts` |

Row format: `["English text", ["ua", "de", ..., "pt"]],` — exactly 14 entries.

## Rules

- Keep `%s` placeholders in every translation, same count.
- Use the language's native quotes (as existing rows do: «…», „…“, ”…”, »…«).
- Prefer a `%s` template over concatenating translated fragments.
- Reuse an existing row if the English text already exists (search first).
- `window.confirm` text goes through `translateMessage`.
- If the English text of an existing string changes, update its row's key.
- Add/extend a case in `web/src/i18n.test.ts` for new templates.
- Build+test: `sh deploy/start.sh local-build`.
