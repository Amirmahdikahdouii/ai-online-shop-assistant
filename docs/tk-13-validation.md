# TK-13 validation — 2026-11-17

Issue: https://github.com/Amirmahdikahdouii/ai-online-shop-assistant/issues/14

## Delivered behavior

- **Precedence-driven product matching**:
  1. Priority order strictly enforced: existing manager or system mapping (`mapping_revision > 0`) -> explicit product/variant code match -> caption and context matching.
  2. Scope restriction: matching candidates and lookup queries are strictly scoped to the same store (`store_id`).
  3. Automatic linking: when a single approved candidate is unambiguously matched without conflict, `match_status='matched'` is set, `mapping_targets` is populated, `mapping_revision=1` is recorded, and an audit row is written to `catalog.post_mapping_changes` with `source='system'`. No duplicate draft product is created.
  4. Ambiguity resolution: unknown code (`unknown_code`), conflicting attributes (`conflicting_attributes`), multiple candidates / multi-product posts (`multiple_candidates`), and similarity without explicit confirmation (`similarity_only`) transition to `match_status='unresolved'`.
  5. In unresolved or non-product states, draft products (`status='draft'`) are preserved for human review. Draft products are never converted to approved catalog items by the system.
  6. Structured audit evidence: `match_evidence` captures the decision reason, Persian summary text, exact quote citations, candidate products/variants, policy version (`matching-v1`), and model identifier.

- **Extractor fixtures**:
  - ScriptedExtractor covers all required scenarios:
    - Same product multiple posts and explicit code (`LIN-01`).
    - Unknown code (`UNK-99`).
    - Similar product with differentiating attributes (`پیراهن لینن نورا آستین کوتاه`).
    - Multi-product post (`ست مانتو لینن نارون و شال نخی ساده`).
    - Conflicting attributes (`شومیز حریر مجلسی`).

- **Dashboard and UI**:
  - Persian status badges: «تطبیق شد» (`match-status-matched`) and «نیاز به بررسی» (`match-status-unresolved`).
  - Plain-language explanation: concise Persian summary text accompanied by an accessible `<details className="match-details">` section containing text quotes, candidate products, policy, and model.
  - Zero misleading numerical or percentage confidence scores.
  - Keyboard/focus accessible summary toggle, RTL styling, local Vazirmatn typography, responsive at 360px and 1280px without horizontal overflow.

## Automated evidence

Database migration upgraded schema version to 11 (`internal/platform/auto_matching.sql` and `internal/platform/platform.go`), adding `match_status` and `match_evidence` to `catalog.post_selections` and enabling `source IN ('manager', 'system')` in `catalog.post_mapping_changes`.

Passed commands:

```sh
go test ./... -count=1
go vet ./...
npm --prefix web run typecheck
npm --prefix web run build
```

Backend integration tests (`internal/identity/matching_test.go`):
- `TestAutoMatchingEngine` runs 10 exhaustive scenarios:
  1. Existing mapping priority over code or caption.
  2. Exact code matching against approved products in same store.
  3. Rejection of unknown codes transitioning to `unresolved`.
  4. Isolation across different stores (store A approved product never matches store B post).
  5. Multi-candidate / multi-product ambiguity handling (`unresolved`).
  6. Conflicting attribute handling (`unresolved`).
  7. Caption similarity rejection when attributes or names differ (`similarity_only` -> `unresolved`).
  8. Non-product post handling (`none`).
  9. Draft product creation on unresolved status for manager review without converting draft to approved.
  10. API retrieval and JSON serialization via `GET /stores/:store/post-selections`.

Browser and smoke validation:
- All 29 Playwright tests passed cleanly (2.2m) against the isolated Docker Compose stack (`shop-smoke-tk13`).
- New test suite `web/tests/matching.spec.ts` exercises:
  - 360px and 1280px viewports.
  - Badge presentation («تطبیق شد» and «نیاز به بررسی»).
  - Evidence inspection via keyboard focus and `<details>` expansion.
  - Complete absence of percentage / confidence scores in user-facing UI.
  - RTL direction and responsive containment.
  - Regression verification across all existing auth, connection, catalog, approval, extraction, post mapping, processing, and outage tests.

## Visual review

Inspected `web/test-results/matching-360.png` and `web/test-results/matching-1280.png`:
- At 360px (mobile): Badges fit naturally beside extraction status headers; details summary expands with quote lists and candidate tags without horizontal page scrolling.
- At 1280px (desktop): Clean layout integration with catalog review actions, accessible contrast ratios, and coherent Persian terminology.

## Reviews and boundaries

- No external or billable APIs were called during test runs; the isolated ScriptedExtractor and local test containers were used exclusively.
- All code respects single-store boundaries, never leaks cross-store data, and preserves human manager overrides.
- Commits are maintained on the local `master` branch and have not been pushed to the remote.
