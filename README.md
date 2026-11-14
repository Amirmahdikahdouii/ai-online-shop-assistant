# Shop assistant · دستیار فروشگاه

TK-01 foundation: Next.js web, Go API/worker, PostgreSQL 17. The Persian RTL start page reports real infrastructure readiness. No business data, external integrations, demo scenarios or authentication are implemented yet.

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
# Replace POSTGRES_PASSWORD with a unique generated secret in the ignored file.
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

Only web is published, on loopback. PostgreSQL/API/worker have no host ports. All demo/live networks and volumes are project-scoped. The browser receives no credential or internal connection URL. Go logs only safe readiness/request IDs, result and duration, never raw database errors/DSNs. Compose configuration and container inspection contain the database credential: restrict access to the Docker daemon and env file. Do not publish `docker compose config` output.

No integration adapter exists in this slice, so neither mode calls external providers. Demo has no live fallback. This is **not a production release or a passed live gate**: public HTTPS, authenticated integrations, account access and backup/restore validation remain later tickets. Do not store valuable live customer data yet.

## Health and migration contract

- `GET /livez` (internal API/worker): process liveness only.
- `GET /readyz` (internal API/worker): 200 only after schema validation, committed isolated write/read probe and a worker heartbeat within 10 seconds; otherwise 503. JSON includes `status`, `mode`, `api`, `database`, `worker`, `probeId`, `checks`, `requestId`. A responding API can be `ready` while the overall system is not ready.
- `GET /api/health` (web): same-origin, uncached server proxy. API unavailability becomes 503 with unknown dependency states; no fabricated healthy fallback.
- The UI checks initially and on explicit retry; its timestamp is the last check, not a continuously refreshed monitor. A stopped worker can remain ready for at most the 10-second heartbeat grace period.
- API and worker attempt migration during startup/recovery. A PostgreSQL advisory transaction lock serializes them; schema version 1 creation is atomic and repeatable. Existing invalid/unsupported schemas fail closed, without destructive repair. Readiness validates required columns/types and actual storage operations, not just `pg_isready`.
- Worker has no business jobs yet: its real duty here is database/schema verification and a heartbeat every 2 seconds. This is not a job-queue implementation.

## Tests

Requires Go 1.26, Node 22+, Docker and Playwright Chromium dependencies:

```sh
go test ./...
go vet ./...
npm --prefix web ci
npm --prefix web exec -- playwright install --with-deps chromium
npm --prefix web run typecheck
bash scripts/smoke.sh
```

Smoke uses a separate `shop-smoke` project/volume and port 3100. Set `SMOKE_PROJECT=shop-smoke-<unique-suffix>` for a fresh database. Its volume is intentionally preserved after testing, never deleted by the script. It tests browser → API → PostgreSQL, write/read, schema damage, worker outage, database outage/recovery, container recreation without lost probe data, local fonts, mobile width, loading/error/empty/success states, keyboard retry and absence of the known server secret in browser assets/logs. Screenshots are in `web/test-results/`.

`HEALTH_BASE_URL` optionally overrides the Playwright origin (default `http://127.0.0.1:3100`) when testing through a local tunnel. Docker builds still require working container DNS/networking for dependency downloads; host networking is not enabled in the shipped configuration.
