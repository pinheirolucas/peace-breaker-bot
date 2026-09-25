---
name: add-endpoint
description: Use when adding or changing an HTTP API route, status code, error label or request/response shape in pkg/server.
---

# Add or change an endpoint

1. Design first. Publish the interface (method, path, body, responses with status + `label`) as an
   artifact and wait for the user's sign-off before writing code.
2. Add the route to `Server.routes()` in `pkg/server/server.go`, and write the handler next to the others.
3. Errors: call `writeErrorMessage(w, status, lang, "<label>")` with a real 4xx/5xx and a stable
   snake_case label. Never fall through to a generic 500 for a known case. Labels are API: the desktop
   branches on them.
4. Add the label's text to both `pkg/i18n/en_us.go` and `pkg/i18n/pt_br.go`.
5. Document the operation in `pkg/server/v1/openapi.yaml`, including an example for every label.
6. Test the handler in `pkg/server/*_test.go` (see `handlers_test.go`, `voice_test.go`).
7. `make test`. `spec_test.go` fails if a route, an operation or a label example is missing from
   openapi.yaml, or if a label has no catalog text.
8. Desktop impact: check `../peace-breaker-bot-desktop/src/service.ts` and its `api.<label>` i18n keys.
   Land this PR first, and open the desktop PR with `Other repo: peace-breaker-bot#N`.
