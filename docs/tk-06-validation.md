# TK-06: durable sample-message ingestion

## Delivered boundaries

- Schema v6 persists normalized inbound events and one job per event. Event insert
  and injected queue enqueue share one PostgreSQL transaction; acceptance is sent
  only after commit. Connection lifecycle and ingestion serialize on the store.
- Connection-scoped external IDs dedupe concurrent/replayed delivery. Events from
  before the saved receiving start are ignored. Sender/page metadata determines
  customer versus shop; the authenticated demo gateway accepts fixed scenarios,
  not arbitrary sender identity, role, body or provider credentials.
- Injected enqueue/claim/ack/retry interface, PostgreSQL `SKIP LOCKED`, 30-second
  leases, fresh claim tokens and lease-expiry fencing. Workers recover expired
  claims. Success and bounded controlled-failure retries persist across processes.
- Protected `/dashboard/processing` shows the newest 100 real events, times,
  speakers, explicit pending/working/success/failure labels, reasons and attempts.
  Poll failures retain the last snapshot; submission errors are independent of
  polling. Replay retains the exact last scenario/key, including failed samples.
- UI reuses local Vazirmatn, light white/slate/blue workspace and RTL alignment.
  A chronological ledger, not JSON or decorative metrics; amber/green/red status
  borders supplement text. Existing focus styles and wrapping controls serve
  keyboard and narrow screens.

## Validation

- Dedicated PostgreSQL 17, no skipped DB tests:
  `REGISTRATION_TEST_DATABASE_URL=… go test -race ./...` passes. New tests cover
  concurrent dedupe, atomic rollback on enqueue failure, DB-side abort before
  commit, old-event filtering, metadata-derived roles, exclusive claim, expired
  lease recovery using a reconstructed queue, stale-token rejection, success,
  three-attempt controlled failure and disconnected rejection. Schema damage to
  the dedupe constraint fails validation. Existing identity/session/store/
  connection tests also pass.
- `go vet ./...` and `npm --prefix web run typecheck` pass.
- Docker production Go/Next.js builds pass.
- Full isolated production Compose smoke passed **17/17 browser tests** (1.4
  minutes): `SMOKE_PROJECT=shop-smoke-tk06-final
  HEALTH_BASE_URL=http://127.0.0.1:3341 bash scripts/smoke.sh`. Includes the new
  flow's loading/empty/pending/success/failure, keyboard submission, exact replay
  for normal/old/failed samples, persistent submission errors, network recovery,
  membership/CSRF/metadata rejection and 360/1280px checks, plus existing
  infrastructure restart/outage coverage. As with TK-05, this host required a
  local TCP tunnel into the web container because direct published-port requests
  returned empty responses. No deployment-network configuration was changed.
  Smoke containers were shut down; test volumes were preserved.
- Visual inspection of `web/test-results/processing-1280.png`,
  `processing-360.png` and `processing-pending.png`: readable Persian text,
  status conveyed by both text and color, wrapping controls, no horizontal
  overflow. Corrected a global paragraph max-width interaction that let the
  first sample button sit beside the explanatory paragraph on desktop.

## Review and limitations

Independent standards and spec reviews found replay identity, malformed scenario
validation, polling clearing submission errors and a racy browser pending check.
These were corrected; the pending browser check pauses/unpauses the worker, and
queue-test schedule mutation is restricted to its own event.

Smoke databases preserve volumes. A processing browser run that is interrupted
before its final disconnect can leave the shared fake page attached; subsequent
runs correctly receive 409. Use a fresh `shop-smoke-<unique-suffix>` project for
an isolated full run rather than deleting existing volumes. The new successful
browser flow explicitly disconnects its fake page when finished.

The Go queue integration test requires an exclusive dedicated test database with
no active worker or unrelated pending jobs, since claims intentionally use the
production global queue. Never point it at a running demo/live database.

Worker recovery is tested at the PostgreSQL lease boundary by expiring a persisted
claim and reconstructing the queue; it does not require waiting 30 seconds or
killing a real process between machine instructions. Delivery is at least once;
this ticket has no non-idempotent external processing effects.

No live gate is claimed. Real BoxAPI authentication/signature verification,
provider callbacks, AI extraction, message bodies, customer/order creation,
historical imports and manual retry are deferred. The live-mode panel may read
statuses, but no live sample-ingestion provider is configured.
