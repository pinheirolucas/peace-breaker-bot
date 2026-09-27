---
name: add-provider
description: Use when adding a new sound site provider under pkg/provider, or when repairing a provider whose site markup changed.
---

# Add or repair a provider

1. Check the site first. Fetch a listing page through `pkg/httpclient` (UA `peace-breaker-bot/1.0`).
   If it answers with a bot wall or a CAPTCHA, stop and tell the user: no proxies, headless browsers
   or evasion.
2. Create `pkg/provider/<key>/<key>.go` implementing `provider.Provider` (`pkg/provider/provider.go`):
   `Key`, `DisplayName`, `List`, `AllowedContentHosts`, `SupportsRegion`. Build requests with
   `httpclient.New()`, never `http.DefaultClient`.
3. `List` returns the sentinel errors from `provider.go` (`ErrInvalidRegion`, `ErrUnexpectedMarkup`,
   `ErrUpstreamUnavailable`, `ErrBadUpstreamStatus`). `server.go` already maps them to statuses.
4. Measure the real page size against live responses and use `provider.InferPages`.
5. Describe the site's markup quirks in the package doc comment (see the existing providers).
6. Capture `testdata/*.html` fixtures live with the app UA and trim them; never hand-write them.
   Cover the full, short and empty pages (see `soundbuttons_test.go`).
7. Register it in `defaultRegistry` in `pkg/server/server.go`, and add the key to the `provider` enum in
   `openapi.yaml` and to the README.
8. `make test`. The desktop picks new providers up from `GET /api/v1/providers`, so it needs no change.
