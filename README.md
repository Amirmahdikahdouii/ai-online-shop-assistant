# Shop assistant · دستیار فروشگاه

Next.js web, Go API/worker, PostgreSQL 17. The Persian RTL start page reports real infrastructure readiness. Accounts register at `/register`, sign in at `/login`, and access the protected `/dashboard`. External integrations and demo scenarios are not implemented yet.

## Run the isolated demo

Requires Docker Engine with Compose, internet for the initial image/build downloads, and a free port 3000.

```sh
docker compose --env-file config/demo.env up --build -d --wait
```

Open <http://localhost:3000>. No OpenAI or BoxAPI credentials are needed. Fonts are bundled locally; runtime makes no external API/CDN requests. A database probe lives exclusively in `infrastructure.health_probe`, not in business tables.

```sh
docker compose --env-file config/demo.env logs -f
docker compose --env-file config/demo.env down
docker compose --env-file config/demo.env up -d --wait
```

`down` preserves the named PostgreSQL volume. **Do not use `down -v`** unless intentionally deleting this environment's data. Image preparation needs internet; subsequent `up --no-build --pull never` uses local images.

## Live configuration (foundation only)

```sh
cp config/live.env.example config/live.env
# Replace POSTGRES_PASSWORD and APP_ORIGIN (your public HTTPS origin).
chmod 600 config/live.env
docker compose --env-file config/live.env up --build -d --wait
```

The example contains no real secrets. Do not reuse the demo password. Keep `APP_MODE=live`, `POSTGRES_DB=shop_live`, and a distinct `COMPOSE_PROJECT_NAME=shop-live`. Startup rejects absent/unknown mode or a database name not matching that mode. Project names scope volumes and networks: never reuse the demo project for live. Changing a password in configuration does not rotate an existing PostgreSQL user's password.

| Setting | Purpose |
| --- | --- |
| `COMPOSE_PROJECT_NAME` | Separate project/network/volume per environment |
| `APP_MODE` | Explicit `demo` or `live`, no fallback |
| `POSTGRES_DB` | `shop_demo` or `shop_live` matching mode |
| `POSTGRES_PASSWORD` | Server-only PostgreSQL credential; never passed to web |
| `WEB_PORT` | Loopback web port, 3000 demo / 3001 live |
| `POSTGRES_PASSWORD_FILE` | Optional API/worker secret-file support for custom deployments |
| `API_URL` | Internal web-server API address, set by Compose |
| `APP_ORIGIN` | Exact browser origin for login/logout CSRF checks; defaults to `http://localhost:3000`. Set the public HTTPS origin in live mode and update it when changing host/port. |

Only web is published, on loopback. PostgreSQL/API/worker have no host ports. All demo/live networks and volumes are project-scoped. The browser receives no credential or internal connection URL. Go logs only safe readiness/request IDs, result and duration, never raw database errors/DSNs. Compose configuration and container inspection contain the database credential: restrict access to the Docker daemon and env file. Do not publish `docker compose config` output.

No integration adapter exists in this slice, so neither mode calls external providers. Demo has no live fallback. This is **not a production release or a passed live gate**: public HTTPS termination, authenticated integrations, store authorization and backup/restore validation remain deployment/later-ticket work. Do not store valuable live customer data yet.

## Health and migration contract

- `GET /livez` (internal API/worker): process liveness only.
- `GET /readyz` (internal API/worker): 200 only after schema validation, committed isolated write/read probe and a worker heartbeat within 10 seconds; otherwise 503. JSON includes `status`, `mode`, `api`, `database`, `worker`, `probeId`, `checks`, `requestId`. A responding API can be `ready` while the overall system is not ready.
- `GET /api/health` (web): same-origin, uncached server proxy. API unavailability becomes 503 with unknown dependency states; no fabricated healthy fallback.
- The UI checks initially and on explicit retry; its timestamp is the last check, not a continuously refreshed monitor. A stopped worker can remain ready for at most the 10-second heartbeat grace period.
- API and worker attempt migration during startup/recovery. A PostgreSQL advisory transaction lock serializes them; schema creation and additive version 1 → 2 → 3 account/session migrations are atomic and repeatable. Existing invalid/unsupported schemas fail closed, without destructive repair. Readiness validates required columns/types and actual storage operations, not just `pg_isready`.
- Worker has no business jobs yet: its real duty here is database/schema verification and a heartbeat every 2 seconds. This is not a job-queue implementation.

## Registration (TK-02)

Open `/register` for the light-only Persian form. Only username and password are
collected. There is deliberately no email, SMS, password recovery or
automatic session creation during registration. Success confirms a committed account,
not an authenticated session or a configured shop.

`POST /api/auth/register` proxies to the private Go `POST /auth/register` API.
The exact normalization, password hashing, response/error contract and pre-build
visual plan are in [docs/tk-02-design.md](docs/tk-02-design.md). Normalized usernames
are unique in PostgreSQL, including concurrent requests; a retry after an unknown
network outcome may report the username already taken, never a second account.
Two concurrent Argon2id hashes per API process are allowed; excess requests receive
429. This is a memory-capacity bound, not a claim of distributed abuse prevention.
Before public deployment add edge abuse/rate controls along with the HTTPS and
session work; the current Compose deployment publishes web only on loopback.

## Login and sessions (TK-03)

`POST /api/auth/login`, `POST /api/auth/logout` and `GET /api/me` proxy to
private Go endpoints. `/me` returns the current user's ID/username, a CSRF token,
and session-scoped store memberships; it never returns
password hashes or session tokens. `/dashboard` is validated against PostgreSQL
on every server request. Only `/dashboard` paths may be resumed after login.

Sessions expire absolutely after seven days. Random opaque tokens are stored only
as SHA-256 hashes in PostgreSQL, survive API restarts, and have no dependency on
page connection state. Logout deletes the server row before clearing the cookie.
Cookies are HttpOnly, SameSite=Lax, host-only, path `/`, and Secure in live mode
(HTTPS required); isolated local demo allows HTTP. Login/logout require an exact
`APP_ORIGIN`; logout additionally requires the per-session `X-CSRF-Token`.
Unknown users and wrong passwords receive the same error and Argon2 work.

PostgreSQL throttles each normalized username to ten attempts per fifteen minutes
and the installation to sixty per minute, including across API restarts/replicas.
Successful authentication clears that account's attempt window (not the global limit).
Two concurrent hashes per API process bound memory. These intentionally conservative
limits can throttle legitimate users under attack; add trusted edge IP controls
before public deployment. Expired attempt/session rows are cleaned on login.
No password recovery or operator reset is provided.

## Store onboarding (TK-04)

Signed-in accounts create their store at `/dashboard`. `POST /api/stores`
requires the session, exact origin and CSRF token. Store and manager membership
commit in one PostgreSQL transaction. Database uniqueness rejects a second store
per user or second manager per store; failed/repeated creates leave no orphan.
User and Store have independently generated opaque IDs. Connection is separate:
no connection record or provider credential is fabricated during onboarding.

`GET /api/me` lists only the session user's store. `GET /api/stores/{id}` returns
404 for both nonexistent and other accounts' stores. Client-supplied store IDs
never grant access. The dashboard has orders, customers, products, attention and
connection routes, collapsible mobile navigation and empty states without fake
counters. Connection settings explicitly report that provider integration is
not implemented yet; empty-state actions navigate there, not to fake operations.

See [TK-04 validation](docs/tk-04-validation.md) for evidence and boundaries.

## Test commands

Requires Go 1.26, Node 22+, Docker and Playwright Chromium dependencies:

```sh
go test ./...
go vet ./...
npm --prefix web ci
npm --prefix web exec -- playwright install --with-deps chromium
npm --prefix web run typecheck
bash scripts/smoke.sh
```

Optional direct Go registration/schema integration tests require a dedicated
PostgreSQL database: set `REGISTRATION_TEST_DATABASE_URL` and run `go test ./...`.
They migrate and create/delete a test account; never point them at live data.
See [TK-02 validation](docs/tk-02-validation.md) for results and screenshot critique.

Smoke uses a separate `shop-smoke` project/volume and port 3100. Set `SMOKE_PROJECT=shop-smoke-<unique-suffix>` for a fresh database. Its volume is intentionally preserved after testing, never deleted by the script. It tests browser → API → PostgreSQL, write/read, schema damage, worker outage, database outage/recovery, container recreation without lost probe data, local fonts, mobile width, loading/error/empty/success states, keyboard retry and absence of the known server secret in browser assets/logs. Screenshots are in `web/test-results/`.

`HEALTH_BASE_URL` optionally overrides the Playwright origin (default `http://127.0.0.1:3100`) when testing through a local tunnel. Docker builds still require working container DNS/networking for dependency downloads; host networking is not enabled in the shipped configuration.
