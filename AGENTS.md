# AGENTS.md

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues on this repo, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

After every ticket implementation, update the originating GitHub issue before the final response. Post a summary of the changes, commit reference(s), tests and their results, and any limitations or remaining work. Update acceptance checkboxes only where supported by evidence. If validation is blocked or incomplete, state that explicitly and leave the issue open; close it only when all acceptance criteria are satisfied. Do not report local commits as pushed or remotely available unless they are.

### Triage labels

Default five-label vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout: root `GLOSSARY.md` + `docs/adr/`. See `docs/agents/domain.md`.
