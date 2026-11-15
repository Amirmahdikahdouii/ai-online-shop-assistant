# TK-12 validation — 2026-11-15

Issue: https://github.com/Amirmahdikahdouii/ai-online-shop-assistant/issues/13

## Delivered behavior

- Store/session-scoped GET and revision-checked POST mapping API. Multiple posts
  reuse the same catalog products; a post can nominate multiple products/variants
  or explicitly be marked as not a product. Product-level links do not infer a
  variant, and several targets never imply ordering all of them.
- Canonical target ordering and identical-save replay avoid extra revisions.
  Foreign-store, missing/archived, wrong-variant, duplicate, overlapping and
  contradictory targets fail without partial writes. Changed stale saves fail.
- Manager ID, source, timestamp and immutable full mapping snapshots are saved
  atomically with the current decision. The change table supplies a durable hook
  for later waiting-work resumption; no resumption worker is implemented here.
- Caption extraction cannot overwrite a manager mapping. If the decision is
  already saved, extraction retains its evidence without creating a competing
  draft. If extraction finishes first, an untouched superseded draft is archived
  with history. Human-edited or otherwise mapped drafts remain available.
- Ordered product locks serialize shared-draft reuse with archival; a later SQL
  statement checks references after any concurrent save has committed.
- Persian keyboard-operable checkboxes, same-store search by name/code/variant,
  explicit saved result and manager history appear beside the saved caption.
  Existing local Vazirmatn, light dashboard styling and RTL are retained.

## Automated evidence

API/worker integration uses a dedicated PostgreSQL 17 container at host port
55413, database `shop_test`, plus the controlled Instagram and extraction
adapters. No real provider account or billable API was used.

Passed commands:

```sh
GOTOOLCHAIN=go1.26.4 REGISTRATION_TEST_DATABASE_URL='<dedicated test database>' go test ./... -count=1
GOTOOLCHAIN=go1.26.4 go vet ./...
npm --prefix web run typecheck
npm --prefix web run build
```

`TestPostMappingLifecycle` verifies persisted multi-target links, product reuse,
repeated saves/reads from a fresh Auth instance, manager corrections, store and
variant rejection, stale revision rejection, and both extraction/decision orders.
A coordinated concurrency test suspends one real API save using a PostgreSQL
trigger/advisory gate, starts competing archival, releases the save, and asserts
through product GET that the shared draft is preserved. This test failed before
the lock fix and passes afterward. The worker duplication assertion also failed
before its fix and passes afterward.

Browser validation runs all 27 Playwright tests against the isolated Compose
stack, including the new two-width mapping flow and a readiness regression that
renames a required mapping audit column, verifies 503, restores it, and verifies
recovery. Mapping checks cover keyboard Space, opening focus, multi-selection,
search empty state, network save failure/retry, saved results after reload,
not-product correction, history, loading, and absence of horizontal overflow.

The environment's default Go 1.26.0 download returned HTTP 403, so tests/builds
used the installed Go 1.26.4 toolchain. Playwright's expected bundled browser was
absent, so a temporary config selected installed `/usr/bin/google-chrome`.
Production web output and the Go binary were packaged into local runtime images;
the shipped Dockerfiles were not changed. Compose service isolation, migrations,
restart persistence and outage recovery were exercised by the full browser suite.
The first full browser attempt lost the exact APP_ORIGIN during the health test's
container recreation; the corrected run propagates APP_ORIGIN and COMPOSE_FILE
through the test process and passes. This was test setup, not a relaxed origin check.

## Visual review

Inspected `web/test-results/post-mapping-360.png` and
`web/test-results/post-mapping-1280.png`. The first pass exposed wrapping between
checkboxes and labels and a short native search field. Labels now stay together
and the input has a 44px minimum height. Final mobile/desktop images show the saved
answer, selection controls and correction history without horizontal overflow.
Generated screenshots and traces remain ignored local test artifacts.

## Reviews and boundaries

Independent standards and spec reviews were run against baseline
`6bc49660288a62ae041a8d0769472d385a8b0781`. Findings about schema validation,
duplicate draft materialization and concurrent shared-draft archival were fixed
and re-reviewed. Both axes have zero remaining material findings.

This ticket does not add automatic matching, waiting-order processing, mutations
of existing orders, or live BoxAPI/OpenAI validation. Archived extraction evidence
is retained. Already human-edited drafts require explicit catalog management.
Commits were made on the existing `master` branch and have not been pushed.

Browser command used the temporary system-Chrome configuration and the local
runtime-image Compose override:

```sh
COMPOSE_FILE=compose.yaml:/tmp/shop-tk12-compose.yaml \
APP_ORIGIN=http://127.0.0.1:3100 SMOKE_PROJECT=shop-smoke-tk12-verified \
npm --prefix web test -- --config=/tmp/shop-tk10-playwright.config.cjs
```

Final result against the `ed8985b` backend: **27 passed (1.4m)**, with zero
remaining review findings and a clean diff check.
