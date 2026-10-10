# ADR-0005: Static SPA embedded in the API image

## Status

Accepted (2026-09-21)

## Context

SnatchArr's UI is an authenticated admin dashboard: no SEO, no public pages. The runtime image is distroless static (praetor ADR-0005), which cannot run Node, and a self-hosted homelab tool benefits from being one container.

## Decision

`web/` builds with `@sveltejs/adapter-static` (`ssr = false`, `fallback: index.html`) and the output is embedded into the Go binary (`api/internal/webui`) and served through golusoris `httpx/static` (immutable caching for `/_app/*`, `no-cache` `index.html` fallback for client routes).

SvelteKit server hooks are therefore unavailable: sessions (`__Host-session` cookie), CSRF (double-submit via `httpx/csrf`) and OIDC redirects live in Go. CSP uses SvelteKit's hash mode in the prerendered `index.html`; Go adds only header-only directives.

## Consequences

- One image, one process, same-origin API calls, no CORS.
- Theme preference is applied client-side (a first-paint flash is accepted).
- `APP_WEB_DEV=1` disables the embedded SPA so Vite's dev server can proxy to the API.

## Notes

2026-10-09: no OIDC login exists yet. The OIDC redirects named above are where that flow
will live once it is built (#26).
