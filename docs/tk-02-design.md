# TK-02 · Registration design (before implementation)

The ticket's light-only direction takes precedence over the unrelated, untracked
dark editorial reference in `DESIGN.md`; that file is left untouched.

## Tokens

- Paper `#f3f6fb`, surface `#ffffff`, ink `#233449`, muted `#566579`,
  primary `#244bc1`, border `#dce3ec`. Error `#a52b32`, success `#176244`.
- Local OFL-licensed Vazirmatn (already bundled via Fontsource), 400/600.
  Body/input 16px, supporting text 14px, heading 32–40px with Persian-friendly
  1.6 line height. No tracking on Persian text.
- Spacing: 8, 12, 16, 24, 32, 48, 64px. Inputs/buttons at least 48px tall;
  input radius 8px, no decorative shadows or gradients.

## Layout and content

RTL, right-aligned text; a quiet header and two unequal columns. On the right,
the actual registration form on white; on the left, a short explanation of the
shop assistant's job and a clearly labelled fictional example of a shop owner's
username (not a pretend live dashboard). No repeated marketing cards, statistics,
unavailable login link or decorative icons. At 360px, one column, 20px gutters,
form before the supporting explanation. Only username and password are collected.

```text
desktop: [ username guidance / product context | heading + form ]
mobile:  [ brand ] [ heading + form ] [ username guidance ]
```

Reviewed against the brief: discarded a dashboard preview because it would imply
features not delivered here. The identity is Persian shop-owner language and a
single useful username example, rather than generic SaaS decoration.

## Form and interaction

- Visible labels, persistent rules and linked field errors. Username direction
  auto-detected (Persian RTL, Latin LTR); password LTR with new-password autocomplete.
- Empty is the initial invitation to create an account, not an infrastructure
  loading screen. No credentials prefilled. Native keyboard submission works.
- A synchronous in-flight guard plus disabled submit prevents double clicks;
  database uniqueness arbitrates simultaneous normalized usernames.
- While sending, label changes and form is busy. Errors never imply success.
  Field errors focus the first invalid field. Network/service errors retain input
  and invite explicit retry; no automatic mutation retry after an ambiguous result.
- Success only after the API commits: replace the form with the returned username,
  clear password, focus confirmation, explain that login is a later slice.
- 3px visible focus outline, no nonessential motion, light theme even with OS dark
  preference. Screenshots: desktop/360px empty, loading, duplicate, error, success.

## Registration contract

`POST /auth/register` internally; same-origin `POST /api/auth/register` in web.
JSON accepts only `username`, `password`. No session is issued in this slice.
Username: trim outer whitespace, NFKC, Unicode case fold, Arabic ي/ى → ی and ك → ک;
then 3–32 Unicode letters/marks/digits or ASCII `_ . -`, with at least one letter
or digit. Internal spaces, controls and bidi overrides are rejected. Store the
normalized username, enforce uniqueness in PostgreSQL. Password: 12–128 Unicode
code points, at most 512 UTF-8 bytes, never trimmed/normalized. Argon2id with a
random 16-byte salt, 64 MiB, 3 passes, parallelism 1, 32-byte output; store PHC
encoding only. Bound concurrent hashing to avoid unbounded memory use.

201 returns opaque user ID, normalized username, RFC3339 creation time and request
ID. Errors: JSON `code`, `fieldErrors`, `requestId`; 409 duplicate, 422 invalid
fields, 400 malformed/unknown JSON, 413 oversized input, 415 non-JSON, 429 hashing
capacity busy, 503 storage unavailable. No password/hash in responses or logs.
Web enforces same-origin mutation with Origin / Fetch Metadata checks and rejects
cross-origin requests; no CORS is enabled. Internal API is not publicly published.
Session/CSRF-cookie lifecycle and login throttling belong to the login ticket.
