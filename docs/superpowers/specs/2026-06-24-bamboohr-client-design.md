# BambooHR Go client — design

**Date:** 2026-06-24
**Status:** Approved (brainstorming) → implementation
**Repo:** `github.com/scjalliance/bamboohr` (new, **private**; OSS-able later)

## Goal

A generic, policy-free Go client for the BambooHR API — auth, transport, typed
errors, retries, pagination, and read access to employee data + metadata. Built
first for SCJ's identity-sync service (which reads employee data from BambooHR as
its authoritative source), but carrying **no SCJ-specific knowledge** so it can be
open-sourced later.

## Scope & boundaries

- **Generic / policy-free.** The client knows the BambooHR API, not SCJ. It has **no**
  knowledge of SCJ custom fields (`customTable####`), the professional-vs-legal name
  rule, `is.active/onboarding` derivation, or timezone policy. All of that lives in
  the consumer (identity-sync). Precedent: `vantagepoint` (generic) vs
  `scj-vantagepoint` (SCJ logic).
- **v1 = read-only**, but **designed for read+write.** The known future write use case
  is recording assigned IT assets in a per-employee **table** (BambooHR "Assets"
  table). The architecture reserves a clean slot for table-row writes; v1 does not
  implement them.
- **Consumer owns semantics.** Field meanings, name rules, and date/timezone business
  rules are the consumer's job (see "Date model" — the client is tz-neutral).

## Module layout

```
github.com/scjalliance/bamboohr            (go 1.26)
├── client.go      Client, New(Config), do() transport (auth, Accept-JSON, retries, errors)
├── config.go      Config + validation
├── employees.go   GetEmployee, ChangedSince
├── datasets.go    Dataset(name) fluent query (the "employee" dataset)
├── meta.go        Fields, Lists, Tables
├── types.go       Record (+ accessors), Date (civil), Field/Table/ChangedEmployee, APIError
├── tables.go      placeholder reserving future per-employee table WRITES (Assets phase)
├── cmd/bamboohr/  small CLI for poking the API (manual smoke, like zohocreator's cmd/zc)
└── *_test.go      httptest-based, table-driven; no live-tenant tests in CI
```

## Transport core

```go
type Config struct {
    APIKey     string       // required; sent as HTTP Basic base64(APIKey + ":x")
    Subdomain  string       // required; company alias, e.g. "scjalliance"
    BaseURL    string       // optional; default "https://api.bamboohr.com/api/gateway.php"
    APIVersion string       // optional; default "v1"
    HTTPClient *http.Client // optional; default with a sane timeout
    UserAgent  string       // optional
    MaxRetries *int         // optional; default 3
}
func New(c Config) (*Client, error)   // validates APIKey + Subdomain
```

`do(ctx, method, path, body, out)` handles, once, for every call:
- **URL:** `{BaseURL}/{Subdomain}/{APIVersion}/{path}`.
- **Auth:** `Authorization: Basic base64(APIKey + ":x")`.
- **`Accept: application/json`** on every request — BambooHR defaults to **XML**; this
  is the classic footgun.
- **Errors:** non-2xx → typed `*APIError{Status int, Message string, BambooHRMessage string}`.
  BambooHR returns detail in the **`X-BambooHR-Error-Message`** header, not the body.
- **Retries + backoff** on `429`/`503`: honor `Retry-After` when present, else capped
  exponential backoff; `MaxRetries` cap; fully `ctx`-cancellable.
- **Pagination:** the datasets API pages differently from legacy reports. The exact
  wire mechanism is **verified during implementation via the `api-explorer` skill**
  before coding; the client exposes an **iterator** that pages transparently so callers
  never handle cursors. (The legacy Make app had a no-pagination truncation bug at
  1000 records — must not recur.)

Every public method takes `ctx context.Context` as its first argument.

## Response model

Employee records are tenant-specific (custom fields vary), so use a dynamic
map-shaped row with typed accessors (house pattern, cf. `zohocreator.Record`):

```go
type Record map[string]any
func (r Record) String(field string) string
func (r Record) Bool(field string) bool
func (r Record) Date(field string) (Date, bool)   // date-only fields
func (r Record) Time(field string) (time.Time, bool) // true timestamps
func (r Record) Has(field string) bool

// Date is a civil (timezone-free) calendar date. Deliberately NOT time.Time:
// a date-only BambooHR value ("2021-12-13") has no instant, and exposing it as
// time.Time would silently imply "midnight UTC". Consumers resolve it to an
// instant under their own business rules.
type Date struct {
    Year  int
    Month time.Month
    Day   int
}

type Field struct { ID, Name, Type, Alias string }
type Table struct { Name, Alias string }
type ChangedEmployee struct { ID string; Action string; LastChanged time.Time } // inserted|updated|deleted
type APIError struct { Status int; Message, BambooHRMessage string }
func (e *APIError) Error() string
```

### Date model (the key boundary)

- **Client is timezone-neutral.** Date-only fields (`hireDate`, `terminationDate`, …)
  → `Date` (civil, no tz). Offset timestamps (`lastChanged`) → `time.Time`. The client
  does **not** impose any timezone (it does **not** replicate the Make app's hand-rolled
  150-line LA-DST converter).
- **Consumer (identity-sync) owns the business meaning.** A date-only value resolves to
  an instant only under business rules: **hireDate → start-of-day (morning), terminationDate
  → 23:59:59 (end-of-day)**, in a chosen timezone. identity-sync v1 applies **US/Pacific
  for all employees** (parity with the current system, simplest), behind a documented seam
  to become **employee-local** later (would need a BambooHR location → IANA-zone map). This
  policy lives in identity-sync, NOT this client.

## Read API (v1)

```go
// datasets.go — fluent; paginates transparently
func (c *Client) Dataset(name string) *DatasetQuery          // e.g. "employee"
func (q *DatasetQuery) Fields(fields ...string) *DatasetQuery
func (q *DatasetQuery) Filter(filters ...Filter) *DatasetQuery
func (q *DatasetQuery) SortBy(field string, desc bool) *DatasetQuery
func (q *DatasetQuery) All(ctx context.Context) ([]Record, error)
func (q *DatasetQuery) Iter(ctx context.Context) iter.Seq2[Record, error]  // Go 1.23 range-over-func

// employees.go
func (c *Client) GetEmployee(ctx context.Context, id string, fields ...string) (Record, error)
func (c *Client) ChangedSince(ctx context.Context, since time.Time) ([]ChangedEmployee, error)

// meta.go
func (c *Client) Fields(ctx context.Context) ([]Field, error)
func (c *Client) Tables(ctx context.Context) ([]Table, error)   // employee tables; Assets lives here
```

The caller names fields explicitly (`Dataset("employee").Fields("customTableNNNN","jobTitle")`);
the client never interprets field names.

## Write path (built 2026-10-02)

Table writes use the v1_1 endpoints; BambooHR deprecated the v1 ones on
2026-07-08.

```go
func (c *Client) AddTableRow(ctx context.Context, employeeID, table string, row Record) error
func (c *Client) UpdateTableRow(ctx context.Context, employeeID, table, rowID string, row Record) error
```

The create returns an empty 200, so `AddTableRow` cannot return the new row id;
callers re-read the table and find the row by a value unique to the add. An add is not
idempotent: it retries only on 429, and any transport error or 5xx wraps
`ErrWriteOutcomeUnknown`.
`Date` marshals as "YYYY-MM-DD".

## Testing

- `httptest`-based, table-driven; no live-tenant tests in CI. Cover:
  - Basic-auth header = `base64(APIKey + ":x")`; `Accept: application/json` always set.
  - Error mapping incl. `X-BambooHR-Error-Message`.
  - Retry/backoff on 429/503 (honors `Retry-After`); `ctx` cancellation.
  - Pagination iterator across ≥2 pages (no truncation).
  - `Record` accessors; **`Date` vs `Time`** parsing (date-only vs offset timestamp).
  - Dataset query serialization (fields/filter/sort) and `ChangedSince` since-param.
- Live smoke only via `cmd/bamboohr` run manually with `envwith -f ~/.secrets/<name>.env -- …`.
- `api-explorer` used during implementation to pin exact schemas + the datasets
  pagination mechanism before coding those calls.

## Project setup (per house conventions)

- Private GitHub repo; `go.mod` module `github.com/scjalliance/bamboohr`, Go 1.26.
- `.secrets/` + `*.env` gitignored; credentials never committed.
- `README.md` (quick start + config) + `docs/`; `AGENTS.md` with `CLAUDE.md` symlink.
- GitHub Actions CI: build, vet, `gofmt -l`, test (race) — mirrors identity-sync.

## Out of scope (v1)

- Writes (employee updates, table rows) — reserved, deferred to the Assets phase.
- Reports API, files, time-off, and other BambooHR domains — add when a consumer needs them.
- Any SCJ-specific field mapping, name rules, or timezone policy — these belong in identity-sync.

## Exit criteria

- `go test ./... -race` green; build/vet/gofmt clean.
- Read flows (dataset employee query w/ pagination, `GetEmployee`, `ChangedSince`,
  `Fields`/`Tables`) work against recorded/httptest fixtures; verified once live via
  `cmd/bamboohr` against the SCJ tenant (read-only).
- No SCJ-specific identifiers anywhere in the module.
