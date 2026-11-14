# TK-02 validation

Scope: [issue #3](https://github.com/Amirmahdikahdouii/ai-online-shop-assistant/issues/3),
registration only. No login/session, email/SMS, recovery, shop setup or live gate
is claimed.

## Results

- `go test ./...`: passed. Without the integration variable, PostgreSQL tests
  explicitly skip; they were also executed separately against the real isolated
  `shop-smoke` PostgreSQL 17 database and **all passed**.
- `REGISTRATION_TEST_DATABASE_URL=<isolated test connection> go test ./... -v`:
  passed, including exact-password Argon2id verification, normalized duplicate
  rejection and schema validation rejecting a dropped username uniqueness
  constraint. Constraint damage is transactional and rolled back.
- `go vet ./...`: passed.
- `npm --prefix web run typecheck`: passed.
- Production Next.js and Go container builds: passed.
- `HEALTH_BASE_URL=http://127.0.0.1:3200 bash scripts/smoke.sh`:
  **8 passed (51.5 seconds)**. Includes existing health/outage/recreation coverage,
  simultaneous normalized registrations yielding exactly 201/409, different salted
  hashes for the same password, Persian Arabic-character equivalence, malformed/
  unknown/oversized payloads, cross-origin rejection, absent recovery endpoints,
  both Persian and Latin usernames at both 360px and 1280px, local fonts, RTL,
  dark OS preference remaining light, keyboard submission/focus, duplicate errors,
  single request on double click, loading, network failure, service failure and
  explicit retry to genuine committed success.

## Visual critique and fixes

Inspected `web/test-results/register-1280-fa-empty.png`, mobile empty,
`register-360-en-duplicate.png`, and `register-360-fa-success.png` directly.
Screenshots for all script/viewport combinations and loading/network error are
generated in ignored `web/test-results/` (local artifacts, not remote attachments).

The right-hand desktop form is visually dominant without repeated decorative
cards. Mobile puts the form before supporting context; labels, persistent hints,
the recovery warning and errors wrap without clipping. The long page scroll is
intentional rather than shrinking Persian body text. Latin input is LTR, Persian
is RTL; the returned username is bidi-isolated. Duplicate error has both text and
border treatment, with a clearly separated blue focus ring. Success remains
quiet and explicitly says login is not delivered yet. No visual clipping was
observed. Review identified a dangling success heading label; it was fixed.

## Review follow-up

Standards: strengthened readiness to validate the username unique constraint;
rejected malformed UTF-8/unpaired-surrogate credentials instead of Go JSON's
silent replacement; added API and PostgreSQL regression tests. Kept the small
identity boundary rather than adding speculative services/repositories.

Spec: expanded the original paired script/viewport coverage to the full 2×2
matrix and repaired the success section's accessible heading reference.

## Environment and boundaries

The ordinary published Docker port returns an empty response on this host,
matching TK-01's documented limitation. All services were healthy; an internal
container request returned 200. A temporary loopback TCP tunnel through
`docker exec` enabled the real host browser/API tests without changing firewall,
Docker daemon, shipped networking or application behavior. The first untunnelled
run failed on connectivity, not acceptance behavior. Browser testing also exposed
and fixed same-origin comparison against Next's internal URL rather than the
request Host. The existing `HEALTH_BASE_URL` override was used for the rerun.

No external-provider access is required or claimed. Hash concurrency is bounded,
not a distributed abuse limiter. Public HTTPS/edge throttling and session security
remain prerequisites of a public deployment/later login work. Demo PostgreSQL
volume is preserved; no user work or unrelated design/skills files were changed.
