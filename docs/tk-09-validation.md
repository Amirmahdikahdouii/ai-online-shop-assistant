# TK-09 / issue #10: browse and select posts

## Delivered

- Provider contract returns page-scoped post IDs, Persian captions, timezone-aware publication times, and cursors. The deterministic fake has eight relevant fixtures, including old posts, and makes no network requests.
- `GET /stores/{store}/posts`, `GET /stores/{store}/post-selections`, and `POST /stores/{store}/post-selections` use session-derived membership. Mutations require the existing Origin/CSRF protections. Provider account/page identities are loaded server-side.
- Schema version 8 stores caption/time snapshots, selection actor/time and processing status. Unique store/connection/post identity and transactional request-key replay prevent duplicate selections, including concurrent requests. Changed payloads with the same key return 409; invalid batches roll back entirely.
- Selected posts start `pending`; extraction, product creation, worker scheduling and product linking are outside this ticket. Existing message ingestion and its old-message cutoff are unchanged. The selection flow never inserts message events.
- Persian RTL/light gallery on the products page reuses the local font. Visible native checkboxes, caption previews, dates, processing labels and a sticky action bar work across pages. Saved selections survive refresh; failed saves preserve unsaved choices and retry keys. Connection-required, loading, empty, error/retry and success states provide explicit actions.
- Existing uncommitted TK-08 catalog work was preserved. Version 8 builds on its version 7 schema.

## Validation (2026-11-14)

- All Go packages passed against a fresh dedicated PostgreSQL 18 database, with race detection:

  ```sh
  REGISTRATION_TEST_DATABASE_URL=postgres://…/posts_validation?sslmode=disable \
    GOTOOLCHAIN=local go test -race -modfile=/tmp/opencode/catalog-test.mod ./...
  ```

  Tests cover adapter contract, single/batch selection, snapshots, duplicate and concurrent requests, idempotency conflict, atomic invalid batches, pagination, fresh Auth instance reads, old posts without message import, foreign-store access/post IDs, anonymous requests, provider errors/empty responses, and stored reads while the provider is unavailable.
- `npm run typecheck`, `npm run build`, and `git diff --check`: passed.
- Playwright `posts.spec.ts`: 2/2 passed using the real API and PostgreSQL, production frontend and installed Chrome, at 360px and 1280px. Covered keyboard Space selection, multi-page selection, failed-save retry, refresh persistence, processing status, CSRF rejection, error/retry, empty state, loading, RTL and no horizontal overflow. Provider empty/error/loading states were controlled through browser routing; normal selection/persistence used real endpoints. Final screenshots at both sizes were visually reviewed; shared header layout collision was fixed.

## Limitations

- Native Go 1.26 execution is blocked: automatic toolchain download returned HTTP 403. Validation used installed Go 1.25.1 with an external temporary modfile changing only the Go directive; repository module files are unchanged. This is compatibility evidence, not native Go 1.26 validation.
- Full isolated Compose smoke and complete browser regression suite were not run. Existing Go regression packages did pass against PostgreSQL.
- No live BoxAPI access, AI extraction, post deselection, or worker processing is claimed. Processing status is persisted as `pending` for a subsequent extraction ticket.
- Keep the issue open pending native-toolchain/full-harness validation.

## Review follow-up (2026-11-15)

- Reviewed both standards/correctness and issue coverage. No blocking post-selection defect was found; three catalog dependency findings were fixed (see TK-08 validation).
- Reran all Go packages against PostgreSQL with race detection (installed Go 1.25.1, external temporary modfile), production build/type checks, and combined catalog/posts Playwright coverage: 4/4 passed at 360px and 1280px.
- Extraction scheduling remains follow-up work, not a claimed part of this ticket's selection persistence. Native Go 1.26/full Compose validation limitations remain unchanged.
