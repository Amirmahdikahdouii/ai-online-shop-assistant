# TK-10 / issue #11: caption extraction into product drafts

## Delivered

- Caption-only `Extractor` interface, typed proposals with known/unknown/conflict fields and caption evidence, and an injected `ScriptedExtractor`. Exact demo-caption fixtures run offline. The input contract has no image/audio attachment, media URL, or arbitrary metadata channel. Scripted extraction is wired only in demo mode; live OpenAI integration remains a separate ticket.
- Schema version 9 adds durable jobs and a unique product link per selected source. Selection and enqueue share a transaction. Migration also enqueues previously pending selections. Claim uses PostgreSQL row locks, lease tokens and expiry; proposal, draft, initial catalog history and job completion commit together. Replays and stale workers cannot create another draft or overwrite subsequent human edits.
- Model output is bounded and validated before applying any part: enums, field cardinality, integer nonnegative safe-range toman prices, source identity and exact caption quotations. Unknown price stays null and conflicting attributes remain unapplied proposals. New products always start as drafts. Proposed codes remain visible for human review without reserving an existing product identity; matching/linking is subsequent work.
- Timeout/provider errors retry with exponential backoff and jitter (three attempts per round by default). Invalid output/refusal fails visibly; unavailable credentials/quota and captions without a supported product block for review. Timeout (default two seconds), lease (30 seconds) and attempt cap are injectable worker/queue settings. Authenticated, membership-scoped and CSRF-protected retry restarts failed/blocked work, never completed work.
- The products page compares the saved caption and extracted fields, with sources beside each value and readable unknown/conflict states. It polls pending work, paginates saved selections and opens the existing editor with keyboard focus. Approval remains an explicit catalog action. The original proposal is labelled as the initial result, distinct from subsequent edits. Existing light colors, RTL and locally hosted Vazirmatn are preserved; mobile uses a single column.

## Validation (2026-11-15)

- API/worker tests against dedicated PostgreSQL 17 passed: successful extraction, repeat selection, one draft, timeout and recovery, invalid enum, foreign source, fabricated quote, negative price, refusal, quota block, unknown price, conflicting attributes, expired lease reclamation, stale/double completion rejection, completed-job retry rejection, and transactional selection rollback when enqueue fails.
- All Go packages passed with race detection against a separate dedicated database:

  ```sh
  REGISTRATION_TEST_DATABASE_URL=postgres://…/shop_tests?sslmode=disable \
    GOTOOLCHAIN=local go test -race -modfile=/tmp/shop-test.mod ./...
  ```

- `npm --prefix web run typecheck`, production build, and `git diff --check` passed.
- Extraction Playwright tests passed 2/2 at 360px and 1280px with production Next.js, real API/worker and PostgreSQL. Selection, resulting draft, evidence, editor handoff, explicit approval and persistence use real endpoints. Pending/failed/conflict/error/loading presentation is controlled through browser routing; corresponding actual failure/recovery semantics are tested at the API/worker boundary. CSRF rejection, keyboard focus, RTL, local font, refresh persistence and lack of horizontal overflow were checked. Both comparison screenshots were visually inspected.
- Full browser suite was attempted: 18 passed, 5 failed on the unavailable Compose smoke harness: infrastructure restart/outage test, two session-expiry tests, worker pause test, and salted-hash SQL inspection test. Failures explicitly report missing `SMOKE_PROJECT` / `shop-smoke-worker-1`, rather than extraction behavior failures.

## Limits

- Go 1.26 toolchain download returned HTTP 403. Tests/build used installed Go 1.25.1 with an external temporary modfile changing only the Go directive. Repository module files remain unchanged. This is compatibility validation, not native Go 1.26 validation.
- Installed Chrome was selected through a temporary Playwright config because the expected bundled Chromium version is absent. Repository Playwright defaults are unchanged.
- Full canonical Compose smoke validation remains incomplete; keep #11 open until that validation is completed. No live AI accuracy, BoxAPI access, multiproduct matching or post-product resolution is claimed.
