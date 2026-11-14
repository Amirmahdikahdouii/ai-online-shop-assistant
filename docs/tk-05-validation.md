# TK-05: simulated Instagram connection

## Delivered boundaries

- Injected Instagram adapter, fixed fake provider identities, demo-only wiring.
  Live mode has no provider and rejects mutations; no external calls or secrets.
- Schema v5 adds connection identity and hashed callback state. Membership,
  origin and CSRF protect lifecycle requests. State expires after ten minutes
  and is scoped to user/store; concurrent callback replay is serialized.
- One retained connection per store; a partial unique index prevents the same
  provider page being connected to two stores. Disconnect releases active-page
  exclusivity but retains identity/start time and invalidates pending callbacks.
  Reconnect fails while another store actively uses the page. Different account
  or page replacement is rejected rather than silently merging namespaces.
- Disconnect never touches sessions or CRM records. There are no CRM business
  records or ingestion jobs yet; this ticket establishes stable scope for them.
- Persian RTL settings reuse the existing white/paper/navy/blue workspace and
  local Vazirmatn, with a clear simulation label, loading/error/retry, keyboard
  controls and mobile layout. Browser screenshots at 360/1280px show readable
  content without horizontal overflow.

## Validation

- Dedicated PostgreSQL 17: `REGISTRATION_TEST_DATABASE_URL=… go test -race ./...`
  passes without skipped database tests. Covers connection, simultaneous callback
  replay, foreign-store rejection, invalid/expired state and callback, disconnect,
  reconnect with unchanged identity/start time, forbidden page replacement,
  cross-store page conflict, transfer after disconnect, handler reconstruction
  (durable state), and absent provider. Existing registration/session/store and
  schema-damage tests also pass.
- `go vet ./...` and `npm --prefix web run typecheck` pass.
- Docker production frontend/backend builds pass. The dedicated connection
  browser test passes, including network failure/retry, keyboard connection,
  replay, disconnect with continued CRM access, reconnect and reload persistence.
- Full production Compose smoke: `SMOKE_PROJECT=shop-smoke-tk05
  HEALTH_BASE_URL=http://127.0.0.1:3338 bash scripts/smoke.sh` passed all **16
  browser tests (1.2 minutes)**, including infrastructure restart/outage checks,
  login/logout and registration database assertions. This environment required
  a local TCP tunnel into the test web container because direct Docker host-port
  requests returned empty responses. The standard smoke project's services were
  shut down by its exit trap; its database volume was retained.

## Review

Independent standards review found no hard documented-rule breaches; noted the
existing compact handler architecture, repeated connection row mapping and a
boolean UI action as nonblocking maintainability opportunities. README drift
was corrected. Spec review found two issues, both fixed: reconnect must use the
saved fixture page, and disconnected pages must not remain permanently reserved.
Database regressions cover page release and reacquisition.

Real OAuth/BoxAPI callbacks, webhook validation, message ingestion and historical
imports are deliberately outside this ticket. The fake callback is an authenticated
same-origin POST, not a claim to implement the real provider's callback contract.
