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
- Mobile evidence and desktop approval/history screenshots were inspected. The
  evidence disclosure wraps long source IDs, forms stack at mobile width, and
  summary/audit contents remain inside the viewport.
- TypeScript typecheck and production Next.js build passed.

## Environment limits

The configured Go 1.26 toolchain download and canonical Docker distroless image
fetch both return HTTP 403 in this environment. Go validation uses installed Go
1.25.1 and an external temporary modfile changing only the Go directive. Tracked
module files and Dockerfiles remain unchanged. Installed Google Chrome is used
through a temporary Playwright configuration. Canonical deployment-image
validation is incomplete; compatibility checks must not be described as native
Go 1.26 or canonical image validation.

Full regression and review results will be recorded after the final checks.
