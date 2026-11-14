# TK-01 validation

Scope: [issue #2](https://github.com/Amirmahdikahdouii/ai-online-shop-assistant/issues/2), parent spec D01/D02/D15, W01 and the infrastructure portion of T20. This does not claim completion of later demo scenarios or any live-provider gate.

## Automated checks

- `go test ./...`: passed (explicit mode/database separation and server credential configuration).
- `go vet ./...`: passed.
- `npm --prefix web run typecheck`: passed.
- Next.js production build: passed, including container build.
- Real PostgreSQL migration on an initially empty volume: all four Compose services became healthy; subsequent startup reuses the schema.
- Full Playwright suite: **2 passed (38.4 seconds)** using the environment accommodations below. API/worker/database outages, schema damage, and exact marker persistence all passed.

## Browser and persistence checks

The Playwright suite covers real browser → Next.js proxy → Go API → PostgreSQL, a committed probe write/read, worker heartbeat expiration/recovery, invalid schema, database/API outages, and recreation of all containers. A random probe-row marker with a fixed value is verified after recreation; a constant ID or a new startup counter alone is not considered proof of persistence.

Desktop and 375px mobile screenshots were inspected: Persian local font, RTL alignment, readable status text, no horizontal overflow, and a visible keyboard-operable retry action. Loading/initial-empty/error states have explicit copy. Screenshots are generated under ignored `web/test-results/`.

## Environment limitation

This host's ordinary Docker build network cannot resolve dependency hosts, and host requests to Docker's published port return an empty response. Inside the web container, requests to both loopback and its container IP return HTTP 200. No host firewall, VPN, DNS or daemon configuration was changed.

Validation therefore uses an **untracked, temporary build-only host-network override**, and a **temporary localhost TCP-over-`docker exec` tunnel** to the actual web container for browser traffic. Runtime services retain their normal project-scoped Compose network. The shipped configuration does not enable host networking or expose PostgreSQL. A default-network fresh-checkout smoke pass on this host is not claimed.

## Review

### Standards

No documented-standard violations. One low-priority duplication finding (unavailable-health contract) was resolved by sharing the typed contract between proxy and UI.

### Spec

Two test findings were resolved: web now has an HTTP healthcheck to avoid startup races, and restart persistence is checked with a unique stored marker rather than only a counter. No business features were added.
