# TK-04: store onboarding and empty workspace

## Design

Light merchant workbench using existing local Vazirmatn. Palette: white
`#ffffff`, paper `#f3f6fb`, navy `#233449`, muted `#566579`, blue `#244bc1`,
line `#dce3ec`. Body 16px, heading 26px (23px mobile), section heading 19px.
Right-aligned RTL sidebar, 240px wide, beside a broad workspace; mobile has an
explicit expandable menu. No totals, graphs or sample orders on an empty database.
One primary action leads to connection prerequisites. Connection reports the
unavailable capability and offers a working return to orders, not a fake operation.

```text
desktop: | broad workspace / title + action | store name / navigation |
mobile:  | store / menu / account |
         | title / action         |
         | empty state or setup   |
```

The untracked DESIGN.md describes a dark editorial landing page; the ticket's
explicit light daily-work panel takes precedence. This plan preserves existing
auth palette/type rather than introducing unrelated marketing styling.

## Validation

- Dedicated PostgreSQL 17: `REGISTRATION_TEST_DATABASE_URL=… go test ./...`
  passes. Store API tests use real sessions/PostgreSQL: two simultaneous first
  creates yield one 201 and one 409, exactly one committed store, independent
  second user, cross-store 404 and database rejection of a second manager.
  Existing registration, session and schema-damage tests also pass.
- `go vet ./...` and `npm --prefix web run typecheck` pass.
- Browser tests in `web/tests/store.spec.ts` cover two accounts at 360/1280px,
  creation, persistence, duplicate POSTs, CSRF, validation, session-derived scope
  despite injected client IDs, anonymous requests, navigation, keyboard/focus,
  mobile overflow, loading, failed fetch/create and retry. Screenshots are local
  ignored artifacts at `web/test-results/store-*.png`.
- Initial smoke run exposed a missing schema-version CHECK update; corrected.
  Subsequent untunnelled browser run failed on host Docker published-port
  connectivity, as previously documented in TK-03. Validation uses the existing
  temporary loopback tunnel on port 3200 with exact APP_ORIGIN, without changing
  shipped networking, firewall or Docker configuration.

## Boundaries

No live provider gate belongs to this ticket. Connection integration, business
records and workers processing store-owned jobs are later slices; no such jobs
exist yet. This does not claim production readiness or implement those later
features. One-store/one-manager limits constrain a separate membership table,
not shared User/Store/Connection IDs.

## Review

Standards review found no documented-standard breaches. Its two correctness
findings were fixed: shared navigation data now lives outside the client module,
and section routing checks own properties (with unknown/constructor/toString
404 regressions). A possible mixed-responsibility smell in the compact identity
store handler is nonblocking and follows the existing handler pattern.

Spec review found no scope creep or implementation defect, with browser/visual
evidence pending at review time. The subsequent full production Compose browser
run passed all **15 tests (1.4 minutes)**. Real PostgreSQL tests also pass under
the race detector. Visual inspection of desktop/mobile screenshots found inherited
global `aside` styling and grid/main auto-spacing adding unnecessary top/vertical
space. Scoped margin/border resets and mobile auto/remaining-space rows correct
those issues. These changes retain the broad workspace and compact merchant
navigation rather than changing the visual direction.
