# TK-08 / issue #9: manual catalog

## Delivered

- Schema version 7 adds store-scoped products, immutable code reservations, and revision snapshots with actor/time. Existing version 6 databases migrate transactionally.
- Authenticated catalog list/detail/create/edit/approve/archive endpoints under `/stores/{store}/products`. Mutations use POST, consistent with the existing session/CSRF proxy; edits and actions require the current revision. Every request checks session-derived membership.
- Products have 1–20 stable variants, optional store-unique codes, attributes, and integer toman prices. Unknown prices remain JSON/DB `null`, including after approval. Codes are trimmed and uppercased before reservation and cannot move to another product/variant, including after archive or code edits.
- Changes and history are committed together. Concurrent code claims yield one success and one conflict, without partial products. Archived products remain readable and cannot be edited.
- Persian RTL dashboard includes manual entry, multiple variants and attributes, explicit save/approval, edit/history, archive confirmation, literal name/variant/code search, status filtering, and 20-item cursor pagination. Unsaved edits disable approval/archive. Loading, empty, error, retry, and conflict recovery are explicit. In-flight mutations are locked against double clicks.

## Validation (2026-11-14)

- `npm ci`, `npm run typecheck`, and `npm run build`: passed.
- Real PostgreSQL 18, fresh isolated database: all Go packages passed with race detection. Command:

  ```sh
  REGISTRATION_TEST_DATABASE_URL=postgres://…/catalog_validation?sslmode=disable \
    GOTOOLCHAIN=local go test -race -modfile=/tmp/opencode/catalog-test.mod ./...
  ```

  The temporary modfile changes only the Go directive from 1.26.0 to installed 1.25.1; repository `go.mod`/`go.sum` are unchanged. Native Go 1.26 validation is **blocked**: automatic toolchain download returned HTTP 403; alternative mirrors failed. This is compatibility evidence, not a claim of running Go 1.26.
- `TestCatalogValidation` and `TestCatalogLifecycle` cover invalid data, creation with known/unknown prices and attributes, duplicate codes, identical codes in separate stores, foreign-store reads/actions/IDs, anonymous access, explicit approval, stale revisions, retained history, archive reservation, absent codes, concurrent claims, literal search, filtering, pagination without duplicates, and reads from a fresh Auth instance.
- Playwright `catalog.spec.ts`: **2/2 passed**, real API/PostgreSQL at 360px and 1280px. Covers create/save/approve/edit, multiple variants, attributes, unknown price persistence, history, stale-revision recovery, search, loading/empty/error/retry, failed save retaining input, double-click suppression, CSRF rejection, archive, and duplicate-code feedback. Checks RTL, keyboard navigation, and no horizontal overflow. Screenshots visually inspected at both sizes.
- Playwright used a temporary external config selecting installed `/usr/bin/google-chrome`; the pinned Playwright browser download was blocked with HTTP 403. Production build was served for these tests; no repo config changes were needed.
- Additional login/register/connection/processing browser regression attempt: 8 passed, 4 not completed successfully. Three require `scripts/smoke.sh`'s isolated Compose harness; the processing case requires a worker (only the catalog API was started). The full Compose/smoke regression was **not validated**.
- `git diff --check`: passed.

## Boundaries and remaining validation

- No post import/linking, AI extraction/matching, or order creation added.
- A variant is the sellable identity; a simple product uses one variant. Existing variants cannot be silently deleted; product archive preserves identity and code reservations.
- Catalog reads are bounded to 20 products per page; product detail returns its revision history.
- Complete the native Go 1.26 and isolated Compose smoke checks before closing the issue.

## Review follow-up (2026-11-15)

- Fixed three review findings: known prices now require an explicitly entered amount instead of defaulting to zero; editable attribute rows retain duplicates until validation rather than overwriting values; catalog requests allow up to 2 MiB in both proxy and API to accommodate the supported form.
- Added duplicate-attribute/value-preservation and explicit-empty-price browser assertions, and a PostgreSQL create/edit regression for a 20-variant Persian product exceeding the former 4 KiB limit.
- All Go packages passed again against PostgreSQL with race detection using installed Go 1.25.1 and the external temporary modfile. Production build/type checks passed. Combined catalog/posts browser suite passed 4/4 at 360px and 1280px; catalog status/retry selectors are scoped to their region. Native Go 1.26 and full Compose validation remain outstanding.
