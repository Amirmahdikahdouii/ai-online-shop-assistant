# TK-11 / issue #12: review and approve product drafts

## Behavior

The existing catalog transaction remains the authority for approval: product and
variant identity are required, price may stay unknown, and the current revision
is checked under a PostgreSQL row lock. Correction and approval each append an
actor/time/snapshot audit record in the same transaction. Drafts remain drafts
until explicit approval; approved status is the catalog eligibility boundary.
Order creation, pending-order reprocessing, post-product matching, and historical
order snapshots belong to subsequent tickets; no order implementation is added.

Product detail now includes saved caption sources and the original structured
proposal, scoped to the authenticated store. Human edits change the product,
not the source evidence. No migration or extraction replay is needed.

The existing light Persian RTL editor with local Vazirmatn now provides:

- A field-by-field before/after summary before saving, and a full saved summary
  before approval, including each variant, code, price and attribute.
- Inline identity, code, price and attribute errors, accessible error references,
  and approval disabled for incomplete or unsaved forms. Unknown price is valid.
- Caption/proposal evidence within the editor, including conflicting/unknown
  proposed values. Manual products explicitly have no saved caption source.
- Immediate audit-history refresh after committed changes, with actor/time and
  attribute snapshots. A failed history refresh is distinct from a failed save.
- Visible revision-conflict feedback without replacing local edits; explicit
  reload obtains the current product and history before retrying.

## Validation

- API/worker tests use a dedicated PostgreSQL 17 instance and controlled caption
  extraction. They cover detail evidence, preservation across correction and
  approval, incomplete identity rejected without revision changes, unknown-price
  approval, stale approval, and concurrent correction/approval where exactly one
  request wins and only one new actor/time audit record is committed.
- The new approval browser test covers the real API lifecycle, inline errors,
  before/after summary, unknown-price approval, a delayed/failed approval request,
  disabled controls during submission, real stale-revision rejection, explicit
  reload, keyboard approval, audit history, and mobile overflow at 360/1280px.
- The existing extraction browser test now checks caption/proposal evidence in
  the editor, correction, approval and persisted original evidence. Together with
  the catalog regression these passed 6/6; the expanded approval checks passed 2/2.
- After review cleanup, the targeted approval/catalog browser regression passed
  **4/4** against the final production build. Mobile editing/validation and
  before/after summaries were also inspected.
- Mobile evidence and desktop approval/history screenshots were inspected. The
  evidence disclosure wraps long source IDs, forms stack at mobile width, and
  summary/audit contents remain inside the viewport.
- TypeScript typecheck, production Next.js build, and `git diff --check` passed.
- All Go packages passed with race detection against a separate dedicated
  PostgreSQL 17 test database; `go vet` passed:

  ```sh
  REGISTRATION_TEST_DATABASE_URL=postgres://…/shop_tests?sslmode=disable \
    GOTOOLCHAIN=local go test -race -modfile=/tmp/shop-test.mod ./...
  GOTOOLCHAIN=local go vet -modfile=/tmp/shop-test.mod ./...
  ```

- The full browser suite passed **25/25**, including database/worker outages,
  schema damage/recovery, container recreation/persistence, session expiry,
  registration, access isolation, processing, post selection and catalog flows.
  The isolated `shop-smoke-tk11` Compose project used the normal service topology
  and PostgreSQL 17, with `/tmp/shop-tk11-compose.yaml` replacing only build/image
  sources: local static Go binary in cached Alpine, and the production standalone
  web output with local Node 26.13.0 in cached Ubuntu. No shipped Compose or
  Dockerfile was changed. This is a fallback-runtime regression result.

  ```sh
  SMOKE_PROJECT=shop-smoke-tk11 \
  COMPOSE_FILE="$PWD/compose.yaml:/tmp/shop-tk11-compose.yaml" \
  APP_ORIGIN=http://127.0.0.1:3100 \
    web/node_modules/.bin/playwright test \
    --config=/tmp/shop-tk10-playwright.config.cjs
  ```

## Environment limits

The configured Go 1.26 toolchain download and canonical Docker distroless image
fetch both return HTTP 403 in this environment. Go validation uses installed Go
1.25.1 and an external temporary modfile changing only the Go directive. Tracked
module files and Dockerfiles remain unchanged. Installed Google Chrome is used
through a temporary Playwright configuration. Canonical deployment-image
validation is incomplete; compatibility checks must not be described as native
Go 1.26 or canonical image validation.

The smoke containers were stopped after validation; their database volume was
preserved. Issue #12 remains open pending canonical toolchain/image validation.
The ticket's API and browser acceptance paths passed in the documented fallback
runtimes; no live integration was required or exercised.

## Standards

No documented-standard violation or blocking finding. One optional duplication
finding (form validation evaluated three times) was addressed by computing it
once. Client validation errors now clear as the field is corrected; a browser
regression checks the accessible invalid state clears after duplicate removal.

## Spec

No missing requirement, scope creep or incorrect behavior found within issue
#12's explicit scope. Caption evidence, required identity, optional unknown
prices, revision arbitration, summaries and explicit approval were reviewed.

Review totals after correction: Standards 0 outstanding; Spec 0 outstanding.
