# Agent Instructions — bamboohr

Generic, policy-free Go client for the BambooHR API (`github.com/scjalliance/bamboohr`).
Built first for the identity-sync service; OSS-able later, so keep it free of any
SCJ-specific identifiers (no custom-field names, no name/timezone business rules —
those belong in the consumer).

## Stack & conventions
- Go 1.26+, stdlib only (no third-party deps). gofmt + Effective Go. Conventional commits.
- HTTP Basic auth (`base64(apiKey + ":x")`); always send `Accept: application/json`.
- Date-only fields use the tz-free `Date` type; never coerce them to an instant here.

## Credentials (local dev)
`~/.secrets/bamboohr.env` (or repo `.secrets/.env`), loaded via
`envwith -f <file> -- <cmd>` (never `source` — fish). Never commit secrets.

## Docs
`README.md` (quick start), `docs/` (design spec + plan).
