# TK-03: login and independent sessions

Scope: [issue #4](https://github.com/Amirmahdikahdouii/ai-online-shop-assistant/issues/4).

## Design

Reuse registration's light-only product language: white form, ink `#233449`,
blue `#244bc1`, muted slate and existing borders, locally bundled Vazirmatn,
persistent labels and the same two-column RTL layout collapsing to one on mobile.
Login's primary action is ورود; registration and login both have explicit password
visibility buttons. Generic credential errors sit beside the password, are
associated with that input, and never identify whether the username exists.
The existing design is intentionally reused, not replaced with a new auth theme.

## Security and boundaries

- Additive schema v2 → v3 creates sessions and shared login-attempt buckets.
- Seven-day absolute sessions use random opaque tokens, SHA-256 storage and
  HttpOnly/SameSite=Lax cookies; live mode additionally requires Secure/HTTPS.
- Origin enforcement protects login/logout; logout requires a session CSRF token.
- PostgreSQL limits survive restarts; account attempts reset on successful login.
  The global limit bounds arbitrary-account traffic, with two local Argon2 slots.
- `/me` validates every session against PostgreSQL; the dashboard does not treat
  cookie presence as authorization. A database outage shows retry guidance rather
  than redirecting repeatedly. Login return paths are restricted to `/dashboard`.
- No provider connection exists in the identity/session schema. No recovery,
  operator reset, email or SMS functionality is introduced. Store memberships are
  not implemented yet; `/me` honestly returns an empty store list.

## Validation

- Real dedicated PostgreSQL 17: Go registration, session, migration and schema
  tests pass. Includes invalid credentials, missing sessions, hashed token storage,
  handler restart persistence, revoked/expired sessions, logout CSRF and throttling.
- `go vet ./...`: passed.
- `npm --prefix web run typecheck`: passed.
- Production Next.js and Go Docker builds: passed.
- Final `HEALTH_BASE_URL=http://127.0.0.1:3200 bash scripts/smoke.sh`:
  **12 passed (1.1 minutes)**, including all existing regression tests.

Browser regressions cover 360px/1280px login, invalid credentials, password
visibility, same-origin CSRF rejection, session persistence across reload, logout,
server-expired and fabricated sessions returning to a stable login URL, preserved
query strings, keyboard submission, loading/duplicate prevention, and network retry.
Existing registration and infrastructure restart/outage suites are retained.

Visual review of the mobile login screenshot identified the global button hover
rule overriding the password toggle's transparent background, reducing text
contrast. A scoped hover rule fixes it. Persistent labels, generic inline errors,
RTL, LTR credentials and the single-column mobile flow remain legible without
horizontal overflow. Screenshots are local ignored Playwright artifacts.

Host Docker published-port requests return empty responses here, as previously
documented for TK-01/TK-02. Browser validation uses the existing temporary loopback
TCP tunnel through `docker exec` on port 3200, with `HEALTH_BASE_URL` and
`APP_ORIGIN` set to that exact origin. No firewall, Docker daemon or shipped
networking changes are needed. The initial untunnelled run failed on connectivity.

## Review

Standards review found normalization/policy/upsert duplication. Registration now
uses the same normalization helper as login; remaining hash-policy and bucket SQL
duplication is local to the small identity boundary, not spread across services.
Spec review identified browser expiry coverage, field-error association, and
success resetting account attempts; regression coverage and behavior were added.

Public HTTPS/edge abuse controls and real provider/disconnect testing remain
deployment/future-provider work, not claims made by this ticket. A targeted account
attack can still temporarily exhaust an account's window; conservative throttling
trades availability for bounded password verification work. This is not a live gate.
