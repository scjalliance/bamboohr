# BambooHR Go Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A generic, policy-free Go client for the BambooHR API — auth, transport, retries, pagination, and read access to employee data + metadata — built for SCJ's identity-sync but carrying no SCJ-specific knowledge.

**Architecture:** One `Client` over a shared `do()` transport (Basic auth, JSON, error mapping, backoff). Domain methods (`employees`, `datasets`, `meta`) sit on top. Records are a dynamic `Record` map with typed accessors; date-only fields use a tz-free `Date` type. Read-only in v1; transport is write-ready for a future Assets-table phase.

**Tech Stack:** Go 1.26, stdlib only (`net/http`, `encoding/json`, `encoding/base64`, `time`, `iter`, `context`, `testing`/`httptest`). No third-party deps.

## Global Constraints

- Module path: `github.com/scjalliance/bamboohr`. Go 1.26+.
- **stdlib only** — no third-party dependencies.
- **Generic / policy-free:** NO SCJ identifiers anywhere (no `customTable####`, no name/timezone rules). The client never interprets field names.
- gofmt-clean; `go vet ./...` clean. Conventional commits.
- Every exported method takes `ctx context.Context` first.
- Transport invariants: `Authorization: Basic base64(APIKey + ":x")`; `Accept: application/json` on every request; retry on **429 and 503** with backoff (honor `Retry-After` if present); non-2xx → `*APIError` capturing the `X-BambooHR-Error-Message` header.
- Date-only fields → tz-free `Date`; never coerced to an instant inside the client.
- Base URL: default `https://{Subdomain}.bamboohr.com` (override via `Config.BaseURL`); request URL = `{BaseURL}/{path}` where callers pass FULL versioned paths (`api/v1/...`, `api/v2/...`, `api/v1_2/...`). There is no `APIVersion` field — endpoints span v1/v1_2/v2, so the version lives in each method's path. (Per the official OpenAPI spec.)

---

### Task 1: Repo bootstrap + CI

**Files:**
- Create: `go.mod`, `.gitignore`, `AGENTS.md`, `CLAUDE.md` (symlink → AGENTS.md), `README.md`, `.github/workflows/ci.yml`, `doc.go`

**Interfaces:**
- Consumes: nothing.
- Produces: a buildable module `github.com/scjalliance/bamboohr` with package `bamboohr`.

- [ ] **Step 1: Init module**

```bash
cd ~/Projects/bamboohr
go mod init github.com/scjalliance/bamboohr
```
Expected: `go.mod` with `module github.com/scjalliance/bamboohr` and `go 1.26`.

- [ ] **Step 2: Write `.gitignore`**

```
# Secrets — never commit
.secrets/
*.env
!*.env.example
# Go build output
/bin/
/dist/
/bamboohr
*.test
*.out
coverage.*
# Credentials
*-credentials.json
gcp-key*.json
# OS / editor
.DS_Store
*.swp
.idea/
.vscode/
```

- [ ] **Step 3: Write `doc.go`** (gives the package a home + a build target before other files exist)

```go
// Package bamboohr is a generic, policy-free Go client for the BambooHR API.
//
// It provides authentication, transport, typed errors, retry/backoff, and
// pagination over BambooHR's REST API, plus read access to employee data and
// metadata. It carries no organization-specific knowledge: callers name the
// fields they want and interpret the values themselves.
package bamboohr
```

- [ ] **Step 4: Write `AGENTS.md` + symlink `CLAUDE.md`**

`AGENTS.md`:
```markdown
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
```
Then:
```bash
ln -sf AGENTS.md CLAUDE.md
```

- [ ] **Step 5: Write `README.md`** (skeleton — quick start filled as methods land)

```markdown
# bamboohr

Generic Go client for the [BambooHR API](https://documentation.bamboohr.com/reference).
Policy-free: you name the fields, you interpret the values. Built for SCJ's
identity-sync; suitable for general use.

## Install

    go get github.com/scjalliance/bamboohr

## Quick start

    c, err := bamboohr.New(bamboohr.Config{APIKey: key, Subdomain: "acme"})
    // ... see docs/ for the full API.

## Docs

- [Design spec](docs/superpowers/specs/2026-06-24-bamboohr-client-design.md)
- [Implementation plan](docs/superpowers/plans/2026-06-24-bamboohr-client.md)
```

- [ ] **Step 6: Write `.github/workflows/ci.yml`**

```yaml
name: ci
on:
  push:
  pull_request:
jobs:
  build-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
          check-latest: true
      - name: gofmt
        run: |
          unformatted=$(gofmt -l .)
          if [ -n "$unformatted" ]; then
            echo "::error::gofmt needed on:"; echo "$unformatted"; exit 1
          fi
      - name: vet
        run: go vet ./...
      - name: build
        run: go build ./...
      - name: test
        run: go test ./... -race
```

- [ ] **Step 7: Verify + commit**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./...`
Expected: builds; vet clean; `gofmt -l` empty; `go test` reports `no test files` (ok).
```bash
git add -A
git commit -m "chore: repo bootstrap — module, package doc, CI, gitignore, agent docs"
```

---

### Task 2: Config + validation

**Files:**
- Create: `config.go`
- Test: `config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Config struct { APIKey, Subdomain, BaseURL, APIVersion, UserAgent string; HTTPClient *http.Client; MaxRetries *int }`
  - `func (c Config) validate() error` (unexported)
  - constants `defaultBaseURL = "https://api.bamboohr.com/api/gateway.php"`, `defaultAPIVersion = "v1"`, `defaultMaxRetries = 3`, `defaultTimeout = 30 * time.Second`.
  - `func (c *Config) applyDefaults()` (unexported): fills BaseURL/APIVersion/HTTPClient/MaxRetries if unset.

- [ ] **Step 1: Write the failing test**

`config_test.go`:
```go
package bamboohr

import "testing"

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"ok", Config{APIKey: "k", Subdomain: "acme"}, false},
		{"missing api key", Config{Subdomain: "acme"}, true},
		{"missing subdomain", Config{APIKey: "k"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.validate(); (err != nil) != tc.wantErr {
				t.Fatalf("validate() err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestConfigApplyDefaults(t *testing.T) {
	c := Config{APIKey: "k", Subdomain: "acme"}
	c.applyDefaults()
	if c.BaseURL != defaultBaseURL {
		t.Fatalf("BaseURL = %q, want default", c.BaseURL)
	}
	if c.APIVersion != defaultAPIVersion {
		t.Fatalf("APIVersion = %q, want %q", c.APIVersion, defaultAPIVersion)
	}
	if c.HTTPClient == nil {
		t.Fatal("HTTPClient not defaulted")
	}
	if c.MaxRetries == nil || *c.MaxRetries != defaultMaxRetries {
		t.Fatalf("MaxRetries not defaulted to %d", defaultMaxRetries)
	}
}

func TestConfigApplyDefaultsKeepsOverrides(t *testing.T) {
	zero := 0
	c := Config{APIKey: "k", Subdomain: "acme", BaseURL: "https://x/", APIVersion: "v1_2", MaxRetries: &zero}
	c.applyDefaults()
	if c.BaseURL != "https://x" { // trailing slash trimmed
		t.Fatalf("BaseURL = %q, want trimmed override", c.BaseURL)
	}
	if c.APIVersion != "v1_2" {
		t.Fatalf("APIVersion overridden away: %q", c.APIVersion)
	}
	if *c.MaxRetries != 0 {
		t.Fatalf("MaxRetries override lost")
	}
}
```

- [ ] **Step 2: Run → fail**

Run: `go test ./... -run TestConfig -v`
Expected: FAIL — undefined `Config`/`validate`/`applyDefaults`/defaults.

- [ ] **Step 3: Write `config.go`**

```go
package bamboohr

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL    = "https://api.bamboohr.com/api/gateway.php"
	defaultAPIVersion = "v1"
	defaultMaxRetries = 3
	defaultTimeout    = 30 * time.Second
)

// Config configures a Client. APIKey and Subdomain are required; the rest default.
type Config struct {
	APIKey     string        // BambooHR API key (sent as Basic base64(APIKey + ":x"))
	Subdomain  string        // company alias, e.g. "acme" in acme.bamboohr.com
	BaseURL    string        // default defaultBaseURL
	APIVersion string        // default "v1"
	UserAgent  string        // optional
	HTTPClient *http.Client  // default: &http.Client{Timeout: defaultTimeout}
	MaxRetries *int          // default 3; pointer so 0 is distinguishable from unset
}

func (c Config) validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("bamboohr: APIKey is required")
	}
	if c.Subdomain == "" {
		return fmt.Errorf("bamboohr: Subdomain is required")
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.APIVersion == "" {
		c.APIVersion = defaultAPIVersion
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: defaultTimeout}
	}
	if c.MaxRetries == nil {
		n := defaultMaxRetries
		c.MaxRetries = &n
	}
}
```

- [ ] **Step 4: Run → pass**

Run: `go test ./... -run TestConfig -v && gofmt -l . && go vet ./...`
Expected: PASS; clean.

- [ ] **Step 5: Commit**

```bash
git add config.go config_test.go
git commit -m "feat: Config with validation + defaults"
```

---

### Task 3: Core types — Record, Date, APIError

**Files:**
- Create: `types.go`
- Test: `types_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Record map[string]any` + methods `String(field) string`, `Bool(field) bool`, `Date(field) (Date, bool)`, `Time(field) (time.Time, bool)`, `Has(field) bool`.
  - `type Date struct { Year int; Month time.Month; Day int }` + `func (d Date) String() string` + `func ParseDate(string) (Date, bool)`.
  - `type Field struct { ID, Name, Type, Alias string }`
  - `type Table struct { Name, Alias string }`
  - `type ChangedEmployee struct { ID, Action string; LastChanged time.Time }`
  - `type APIError struct { Status int; Message, BambooHRMessage string }` + `Error() string`.

- [ ] **Step 1: Write the failing test**

`types_test.go`:
```go
package bamboohr

import (
	"testing"
	"time"
)

func TestRecordAccessors(t *testing.T) {
	r := Record{
		"firstName": "Jane",
		"isActive":  "true",
		"count":     float64(3), // JSON numbers decode to float64
		"hireDate":  "2021-12-13",
		"changed":   "2026-06-23T20:21:20+00:00",
		"empty":     "",
	}
	if r.String("firstName") != "Jane" {
		t.Fatalf("String = %q", r.String("firstName"))
	}
	if r.String("count") != "3" {
		t.Fatalf("String(count) = %q, want 3", r.String("count"))
	}
	if !r.Bool("isActive") {
		t.Fatal("Bool(isActive) = false")
	}
	if r.Bool("missing") {
		t.Fatal("Bool(missing) = true, want false")
	}
	if !r.Has("empty") || r.Has("nope") {
		t.Fatal("Has wrong")
	}
	d, ok := r.Date("hireDate")
	if !ok || d.Year != 2021 || d.Month != time.December || d.Day != 13 {
		t.Fatalf("Date(hireDate) = %+v ok=%v", d, ok)
	}
	if _, ok := r.Date("changed"); ok {
		t.Fatal("Date should not parse a full timestamp as a civil date")
	}
	ts, ok := r.Time("changed")
	if !ok || ts.Year() != 2026 || ts.Hour() != 20 {
		t.Fatalf("Time(changed) = %v ok=%v", ts, ok)
	}
}

func TestDateString(t *testing.T) {
	if got := (Date{2021, time.December, 13}).String(); got != "2021-12-13" {
		t.Fatalf("Date.String = %q", got)
	}
}

func TestParseDateRejectsTimestamp(t *testing.T) {
	if _, ok := ParseDate("2026-06-23T20:21:20+00:00"); ok {
		t.Fatal("ParseDate accepted a timestamp")
	}
	if _, ok := ParseDate(""); ok {
		t.Fatal("ParseDate accepted empty")
	}
	if d, ok := ParseDate("2021-12-13"); !ok || d.Day != 13 {
		t.Fatalf("ParseDate(date) = %+v ok=%v", d, ok)
	}
}

func TestAPIError(t *testing.T) {
	e := &APIError{Status: 403, Message: "Forbidden", BambooHRMessage: "no field access"}
	if e.Error() == "" {
		t.Fatal("APIError.Error empty")
	}
}
```

- [ ] **Step 2: Run → fail**

Run: `go test ./... -run 'TestRecord|TestDate|TestParseDate|TestAPIError' -v`
Expected: FAIL — undefined types.

- [ ] **Step 3: Write `types.go`**

```go
package bamboohr

import (
	"fmt"
	"strconv"
	"time"
)

// Record is a dynamic BambooHR record: field name → value. BambooHR field sets
// are tenant-specific (custom fields), so records are maps with typed accessors
// rather than fixed structs. JSON numbers arrive as float64; most values are strings.
type Record map[string]any

// Has reports whether the field is present.
func (r Record) Has(field string) bool { _, ok := r[field]; return ok }

// String returns the field as a string ("" if absent). Non-strings are formatted.
func (r Record) String(field string) string {
	v, ok := r[field]
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(s)
	default:
		return fmt.Sprintf("%v", s)
	}
}

// Bool returns the field as a bool (false if absent/unparseable).
func (r Record) Bool(field string) bool {
	switch v := r[field].(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	default:
		return false
	}
}

// Date returns a date-only field as a civil Date. ok is false if absent or if the
// value is not a bare YYYY-MM-DD (e.g. a full timestamp is rejected — use Time).
func (r Record) Date(field string) (Date, bool) {
	return ParseDate(r.String(field))
}

// Time returns a timestamp field parsed as RFC3339/ISO-8601. ok is false otherwise.
func (r Record) Time(field string) (time.Time, bool) {
	s := r.String(field)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// Date is a civil (timezone-free) calendar date. It is deliberately NOT time.Time:
// a date-only BambooHR value has no instant, and the business meaning (start vs end
// of day, in which timezone) belongs to the caller.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// ParseDate parses a bare "YYYY-MM-DD". It rejects empty strings and anything with
// a time component (so timestamps don't masquerade as dates).
func ParseDate(s string) (Date, bool) {
	if len(s) != len("2006-01-02") {
		return Date{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, false
	}
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}, true
}

// Field is a BambooHR field definition (from meta/dataset-fields).
type Field struct {
	ID    string
	Name  string
	Type  string
	Alias string
}

// Table is a BambooHR employee table definition (e.g. the Assets table).
type Table struct {
	Name  string
	Alias string
}

// ChangedEmployee is one entry from the changed-employees endpoint.
type ChangedEmployee struct {
	ID          string
	Action      string // "Inserted" | "Updated" | "Deleted"
	LastChanged time.Time
}

// APIError is a non-2xx BambooHR response. BambooHR puts human detail in the
// X-BambooHR-Error-Message response header rather than the body.
type APIError struct {
	Status          int
	Message         string
	BambooHRMessage string
}

func (e *APIError) Error() string {
	if e.BambooHRMessage != "" {
		return fmt.Sprintf("bamboohr: HTTP %d: %s (%s)", e.Status, e.Message, e.BambooHRMessage)
	}
	return fmt.Sprintf("bamboohr: HTTP %d: %s", e.Status, e.Message)
}
```

- [ ] **Step 4: Run → pass**

Run: `go test ./... -run 'TestRecord|TestDate|TestParseDate|TestAPIError' -v && gofmt -l . && go vet ./...`
Expected: PASS; clean.

- [ ] **Step 5: Commit**

```bash
git add types.go types_test.go
git commit -m "feat: core types — Record accessors, civil Date, APIError"
```

---

### Task 4: Client transport (`do`) — auth, JSON, errors, retry/backoff

**Files:**
- Create: `client.go`
- Test: `client_test.go`

**Interfaces:**
- Consumes: `Config` (Task 2), `APIError` (Task 3).
- Produces:
  - `type Client struct { cfg Config; baseDelay time.Duration }`
  - `func New(c Config) (*Client, error)`
  - `func (c *Client) do(ctx context.Context, method, path string, body any, out any) error` (unexported) — builds URL `{BaseURL}/{Subdomain}/{APIVersion}/{path}`, sets auth + `Accept: application/json` (+ `Content-Type` when body), retries 429/503, decodes JSON into `out`, maps errors to `*APIError`.
  - `func (c *Client) get(ctx, path string, query url.Values, out any) error` helper.

- [ ] **Step 1: Write the failing test**

`client_test.go`:
```go
package bamboohr

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := New(Config{APIKey: "key", Subdomain: "acme", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.baseDelay = time.Millisecond // keep retry tests fast
	return c
}

func TestDoSetsAuthAndAccept(t *testing.T) {
	var gotAuth, gotAccept, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotPath = r.URL.Path
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	var out map[string]any
	if err := c.do(context.Background(), http.MethodGet, "ping", nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("key:x"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
	if gotAccept != "application/json" {
		t.Fatalf("Accept = %q", gotAccept)
	}
	if gotPath != "/acme/v1/ping" {
		t.Fatalf("path = %q, want /acme/v1/ping", gotPath)
	}
	if out["ok"] != true {
		t.Fatalf("decode failed: %v", out)
	}
}

func TestDoMapsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-BambooHR-Error-Message", "no field access")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := testClient(t, srv)

	err := c.do(context.Background(), http.MethodGet, "x", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.Status != 403 || apiErr.BambooHRMessage != "no field access" {
		t.Fatalf("APIError = %+v", apiErr)
	}
}

func TestDoRetriesOn503Then200(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	if err := c.do(context.Background(), http.MethodGet, "x", nil, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (2 retries)", calls)
	}
}

func TestDoRetriesExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := testClient(t, srv)

	err := c.do(context.Background(), http.MethodGet, "x", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 {
		t.Fatalf("err = %v, want *APIError 429 after retries", err)
	}
}

func TestDoSendsJSONBody(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
		gotBody = string(b)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	body := map[string]any{"fields": []string{"firstName"}}
	if err := c.do(context.Background(), http.MethodPost, "x", body, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
	if !strings.Contains(gotBody, `"fields"`) {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestDoContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // would retry forever-ish
	}))
	defer srv.Close()
	c := testClient(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.do(ctx, http.MethodGet, "x", nil, nil); err == nil {
		t.Fatal("expected context error")
	}
}
```

- [ ] **Step 2: Run → fail**

Run: `go test ./... -run TestDo -v`
Expected: FAIL — undefined `New`/`do`.

- [ ] **Step 3: Write `client.go`**

```go
package bamboohr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client talks to the BambooHR API.
type Client struct {
	cfg       Config
	baseDelay time.Duration // base backoff between retries (overridable in tests)
}

// New validates the config, applies defaults, and returns a Client.
func New(c Config) (*Client, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	c.applyDefaults()
	return &Client{cfg: c, baseDelay: 500 * time.Millisecond}, nil
}

func (c *Client) authHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.cfg.APIKey+":x"))
}

func (c *Client) urlFor(path string) string {
	return fmt.Sprintf("%s/%s/%s/%s", c.cfg.BaseURL, c.cfg.Subdomain, c.cfg.APIVersion, path)
}

// get is a convenience for GET with query params.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// do performs an authenticated JSON request with retry/backoff on 429/503 and
// decodes a 2xx JSON body into out (if non-nil). Non-2xx → *APIError.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		if bodyBytes, err = json.Marshal(body); err != nil {
			return fmt.Errorf("bamboohr: marshal body: %w", err)
		}
	}

	maxAttempts := *c.cfg.MaxRetries + 1
	var lastErr error
	var delayHint time.Duration // Retry-After from the previous attempt, if any
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if err := c.wait(ctx, attempt, delayHint); err != nil {
				return err
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, c.urlFor(path), bytesReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("bamboohr: new request: %w", err)
		}
		req.Header.Set("Authorization", c.authHeader())
		req.Header.Set("Accept", "application/json")
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.cfg.UserAgent != "" {
			req.Header.Set("User-Agent", c.cfg.UserAgent)
		}

		resp, err := c.cfg.HTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("bamboohr: %s %s: %w", method, path, err)
		}

		// 429 (too fast) and 503 (gateway overwhelmed) are both retryable.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			delayHint = parseRetryAfter(resp.Header.Get("Retry-After"))
			lastErr = &APIError{Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode),
				BambooHRMessage: resp.Header.Get("X-BambooHR-Error-Message")}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return &APIError{Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode),
				BambooHRMessage: resp.Header.Get("X-BambooHR-Error-Message")}
		}
		if out == nil {
			io.Copy(io.Discard, resp.Body)
			return nil
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("bamboohr: decode response: %w", err)
		}
		return nil
	}
	return lastErr // retries exhausted → the last rate-limit *APIError
}

// wait sleeps before a retry: the Retry-After hint if present, else capped
// exponential backoff (baseDelay × 2^(attempt-1)). Cancellable via ctx.
func (c *Client) wait(ctx context.Context, attempt int, hint time.Duration) error {
	d := hint
	if d <= 0 {
		d = c.baseDelay * time.Duration(1<<uint(attempt-1)) // 1x, 2x, 4x, ...
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func bytesReader(b []byte) io.Reader {
	if b == nil {
		return nil
	}
	return bytes.NewReader(b)
}

func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil {
		return time.Duration(secs) * time.Second
	}
	return 0
}
```

- [ ] **Step 4: Run → pass**

Run: `go test ./... -run TestDo -v && gofmt -l . && go vet ./...`
Expected: PASS; clean.

- [ ] **Step 5: Commit**

```bash
git add client.go client_test.go
git commit -m "feat: Client transport — Basic auth, JSON, error mapping, retry/backoff"
```

---

### Task 5: Metadata — `Fields`, `Tables`

**Files:**
- Create: `meta.go`
- Test: `meta_test.go`

**Interfaces:**
- Consumes: `Client.get` (Task 4), `Field`/`Table` (Task 3).
- Produces:
  - `func (c *Client) Fields(ctx context.Context) ([]Field, error)` — GET `api/v1/meta/fields`.
  - `func (c *Client) Tables(ctx context.Context) ([]Table, error)` — GET `api/v1/meta/tables`.
  - Note: `meta/tables` response per spec is an array of `{ alias, fields: [{id,name,alias,type}] }`; tables are keyed by `alias` and `name` may be absent. `Table{Name, Alias}` tolerates a missing `name`.

- [ ] **Step 1: Write the failing test**

`meta_test.go`:
```go
package bamboohr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/meta/fields" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"id":1234,"name":"Display Name","type":"text","alias":"displayName"},
		                 {"id":"firstName","name":"First Name","type":"text"}]`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	fields, err := c.Fields(context.Background())
	if err != nil {
		t.Fatalf("Fields: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("len = %d, want 2", len(fields))
	}
	if fields[0].ID != "1234" || fields[0].Alias != "displayName" {
		t.Fatalf("field[0] = %+v (id should normalize numeric→string)", fields[0])
	}
	if fields[1].ID != "firstName" {
		t.Fatalf("field[1].ID = %q", fields[1].ID)
	}
}

func TestTables(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/meta/tables" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"alias":"customAssets","name":"Assets"}]`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	tables, err := c.Tables(context.Background())
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if len(tables) != 1 || tables[0].Alias != "customAssets" {
		t.Fatalf("tables = %+v", tables)
	}
}
```

- [ ] **Step 2: Run → fail**

Run: `go test ./... -run 'TestFields|TestTables' -v`
Expected: FAIL — undefined `Fields`/`Tables`.

- [ ] **Step 3: Write `meta.go`**

```go
package bamboohr

import (
	"context"
	"encoding/json"
)

// Fields returns every field available to the API key.
func (c *Client) Fields(ctx context.Context) ([]Field, error) {
	// BambooHR field IDs may be numeric or string; decode loosely then normalize.
	var raw []struct {
		ID    json.RawMessage `json:"id"`
		Name  string          `json:"name"`
		Type  string          `json:"type"`
		Alias string          `json:"alias"`
	}
	if err := c.get(ctx, "api/v1/meta/fields", nil, &raw); err != nil {
		return nil, err
	}
	fields := make([]Field, 0, len(raw))
	for _, f := range raw {
		fields = append(fields, Field{ID: jsonScalarString(f.ID), Name: f.Name, Type: f.Type, Alias: f.Alias})
	}
	return fields, nil
}

// Tables returns the employee table definitions (e.g. the Assets table).
// Per spec the response is an array of { alias, fields: [...] }; name may be absent.
// Alias is the stable key; Name is informational only.
func (c *Client) Tables(ctx context.Context) ([]Table, error) {
	var raw []struct {
		Name  string `json:"name"`
		Alias string `json:"alias"`
	}
	if err := c.get(ctx, "api/v1/meta/tables", nil, &raw); err != nil {
		return nil, err
	}
	tables := make([]Table, 0, len(raw))
	for _, t := range raw {
		tables = append(tables, Table{Name: t.Name, Alias: t.Alias})
	}
	return tables, nil
}

// jsonScalarString renders a JSON scalar (string or number) as a string,
// trimming surrounding quotes from string-encoded values.
func jsonScalarString(raw json.RawMessage) string {
	s := string(raw)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			return str
		}
	}
	return s
}
```

- [ ] **Step 4: Run → pass**

Run: `go test ./... -run 'TestFields|TestTables' -v && gofmt -l . && go vet ./...`
Expected: PASS; clean.

- [ ] **Step 5: Commit**

```bash
git add meta.go meta_test.go
git commit -m "feat: meta — Fields and Tables"
```

---

### Task 6: Employees — `GetEmployee`, `ChangedSince`

**Files:**
- Create: `employees.go`
- Test: `employees_test.go`

**Interfaces:**
- Consumes: `Client.get` (Task 4), `Record`/`ChangedEmployee` (Task 3).
- Produces:
  - `func (c *Client) GetEmployee(ctx context.Context, id string, fields ...string) (Record, error)` — GET `api/v1/employees/{id}?fields=...&onlyCurrent=true`.
  - `func (c *Client) ChangedSince(ctx context.Context, since time.Time) ([]ChangedEmployee, error)` — GET `api/v1/employees/changed?since=<RFC3339>`.
  - Note: `ChangedSince` response includes a top-level `latest` field alongside `employees`; it is ignored. Action values are Title-case as returned by the API: `Inserted`, `Updated`, `Deleted` — passed through as-is without normalization.

- [ ] **Step 1: Write the failing test**

`employees_test.go`:
```go
package bamboohr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetEmployee(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/employees/42" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("fields") != "firstName,lastName" {
			t.Errorf("fields = %q", r.URL.Query().Get("fields"))
		}
		w.Write([]byte(`{"id":"42","firstName":"Jane","lastName":"Doe"}`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	rec, err := c.GetEmployee(context.Background(), "42", "firstName", "lastName")
	if err != nil {
		t.Fatalf("GetEmployee: %v", err)
	}
	if rec.String("id") != "42" || rec.String("firstName") != "Jane" {
		t.Fatalf("rec = %+v", rec)
	}
}

func TestChangedSince(t *testing.T) {
	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/employees/changed" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("since"); got != since.Format(time.RFC3339) {
			t.Errorf("since = %q", got)
		}
		// Response includes top-level "latest" field (ignored) alongside "employees".
		// Action values are Title-case as returned by the API.
		w.Write([]byte(`{"latest":"2026-06-23T20:21:20+00:00","employees":{
			"42":{"id":"42","action":"Updated","lastChanged":"2026-06-23T20:21:20+00:00"},
			"7":{"id":"7","action":"Inserted","lastChanged":"2026-06-22T10:00:00+00:00"}}}`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	changed, err := c.ChangedSince(context.Background(), since)
	if err != nil {
		t.Fatalf("ChangedSince: %v", err)
	}
	if len(changed) != 2 {
		t.Fatalf("len = %d, want 2", len(changed))
	}
	byID := map[string]ChangedEmployee{}
	for _, ce := range changed {
		byID[ce.ID] = ce
	}
	if byID["42"].Action != "Updated" || byID["42"].LastChanged.Hour() != 20 {
		t.Fatalf("changed[42] = %+v", byID["42"])
	}
	if byID["7"].Action != "Inserted" {
		t.Fatalf("changed[7].Action = %q, want Inserted", byID["7"].Action)
	}
}
```

- [ ] **Step 2: Run → fail**

Run: `go test ./... -run 'TestGetEmployee|TestChangedSince' -v`
Expected: FAIL — undefined methods.

- [ ] **Step 3: Write `employees.go`**

```go
package bamboohr

import (
	"context"
	"net/url"
	"strings"
	"time"
)

// GetEmployee fetches one employee's requested fields. id is the BambooHR
// employee id. At least one field should be requested.
func (c *Client) GetEmployee(ctx context.Context, id string, fields ...string) (Record, error) {
	q := url.Values{}
	if len(fields) > 0 {
		q.Set("fields", strings.Join(fields, ","))
	}
	q.Set("onlyCurrent", "true")
	var rec Record
	if err := c.get(ctx, "api/v1/employees/"+url.PathEscape(id), q, &rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// ChangedSince returns employees changed at/after since. Action values are
// Title-case as returned by the API (Inserted, Updated, Deleted); passed through as-is.
func (c *Client) ChangedSince(ctx context.Context, since time.Time) ([]ChangedEmployee, error) {
	q := url.Values{}
	q.Set("since", since.Format(time.RFC3339))
	var resp struct {
		Latest    string `json:"latest"` // ignored; tolerated to avoid decode noise
		Employees map[string]struct {
			ID          string `json:"id"`
			Action      string `json:"action"`
			LastChanged string `json:"lastChanged"`
		} `json:"employees"`
	}
	if err := c.get(ctx, "api/v1/employees/changed", q, &resp); err != nil {
		return nil, err
	}
	out := make([]ChangedEmployee, 0, len(resp.Employees))
	for key, e := range resp.Employees {
		id := e.ID
		if id == "" {
			id = key
		}
		ce := ChangedEmployee{ID: id, Action: e.Action}
		if t, err := time.Parse(time.RFC3339, e.LastChanged); err == nil {
			ce.LastChanged = t
		}
		out = append(out, ce)
	}
	return out, nil
}
```

- [ ] **Step 4: Run → pass**

Run: `go test ./... -run 'TestGetEmployee|TestChangedSince' -v && gofmt -l . && go vet ./...`
Expected: PASS; clean.

- [ ] **Step 5: Commit**

```bash
git add employees.go employees_test.go
git commit -m "feat: employees — GetEmployee and ChangedSince"
```

---

### Task 7: Datasets — fluent query with transparent pagination

**Files:**
- Create: `datasets.go`
- Test: `datasets_test.go`

**Interfaces:**
- Consumes: `Client.do` (Task 4), `Record` (Task 3).
- Produces:
  - `func (c *Client) Dataset(name string) *DatasetQuery`
  - `func (q *DatasetQuery) Fields(fields ...string) *DatasetQuery`
  - `func (q *DatasetQuery) Filter(odata string) *DatasetQuery` — stores a raw OData filter string; multiple calls concatenate with `" and "`.
  - `func (q *DatasetQuery) SortBy(field string, desc bool) *DatasetQuery` — appends to a comma-joined `orderBy` string (e.g. `lastChanged desc`); every field in orderBy must also be in fields.
  - `func (q *DatasetQuery) PageSize(n int) *DatasetQuery`
  - `func (q *DatasetQuery) All(ctx context.Context) ([]Record, error)` — pages using `meta.totalPages` from the response.
  - `func (q *DatasetQuery) Iter(ctx context.Context) iter.Seq2[Record, error]` — Go 1.23 range-over-func; stops when the consumer breaks.

> Wire format (confirmed by the official OpenAPI spec):
> `POST api/v2/datasets/{datasetName}/data` body `{"fields":[...],"filter":"<odata>","orderBy":"<field> asc|desc,...","page":N,"pageSize":M}` (note: `pageSize` camelCase, not `page_size`).
> Response: `{"data":[{"fields":{<fieldName>:value,...}},...], "links":{"next":"...","prev":"..."}, "meta":{"page":N,"pageSize":M,"totalPages":N,"totalItems":N}}`.
> Each row's values are nested under a `fields` key. Pagination stops when `page >= meta.totalPages` (or `data` is empty).
> A live smoke via `cmd/bamboohr` after Task 8 is still worth doing to confirm against the real tenant.

- [ ] **Step 0: Wire format confirmed by spec**

The v2 shape (`POST .../data`, `pageSize` camelCase, rows under `"fields"`, `meta.totalPages`) is confirmed by the official OpenAPI spec. No live call is needed to unblock implementation. After Task 8 is complete, a single read-only smoke call is still recommended to confirm behavior against the real tenant:

```bash
cd ~/Projects/bamboohr
envwith -f ~/.secrets/bamboohr.env -- go run ./cmd/bamboohr dataset employee --fields id,lastChanged --page-size 1
```

- [ ] **Step 1: Write the failing test**

`datasets_test.go`:
```go
package bamboohr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDatasetAllPaginates(t *testing.T) {
	var pages []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/datasets/employee/data" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Fields   []string `json:"fields"`
			Page     int      `json:"page"`
			PageSize int      `json:"pageSize"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		pages = append(pages, body.Page)
		if body.PageSize != 2 {
			t.Errorf("pageSize = %d, want 2", body.PageSize)
		}
		// 3 total items across 2 pages (totalPages=2).
		switch body.Page {
		case 1:
			fmt.Fprint(w, `{"data":[{"fields":{"id":"1"}},{"fields":{"id":"2"}}],"meta":{"page":1,"pageSize":2,"totalPages":2,"totalItems":3}}`)
		case 2:
			fmt.Fprint(w, `{"data":[{"fields":{"id":"3"}}],"meta":{"page":2,"pageSize":2,"totalPages":2,"totalItems":3}}`)
		default:
			fmt.Fprint(w, `{"data":[],"meta":{"page":3,"pageSize":2,"totalPages":2,"totalItems":3}}`)
		}
	}))
	defer srv.Close()
	c := testClient(t, srv)

	recs, err := c.Dataset("employee").Fields("id").SortBy("lastChanged", true).PageSize(2).All(context.Background())
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("len = %d, want 3 (no truncation across pages)", len(recs))
	}
	if len(pages) != 2 || pages[0] != 1 || pages[1] != 2 {
		t.Fatalf("pages requested = %v, want [1 2]", pages)
	}
	if recs[2].String("id") != "3" {
		t.Fatalf("recs[2] = %+v", recs[2])
	}
}

func TestDatasetIterEarlyStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always returns a full page with totalPages=99 → would page forever without early stop.
		fmt.Fprint(w, `{"data":[{"fields":{"id":"1"}},{"fields":{"id":"2"}}],"meta":{"page":1,"pageSize":2,"totalPages":99,"totalItems":198}}`)
	}))
	defer srv.Close()
	c := testClient(t, srv)

	var seen []string
	for rec, err := range c.Dataset("employee").Fields("id").PageSize(2).Iter(context.Background()) {
		if err != nil {
			t.Fatalf("iter err: %v", err)
		}
		seen = append(seen, rec.String("id"))
		if len(seen) == 3 { // break mid-stream
			break
		}
	}
	if len(seen) != 3 {
		t.Fatalf("seen = %v, want 3 then break", seen)
	}
}

func TestDatasetSerializesFilterAndSort(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		fmt.Fprint(w, `{"data":[],"meta":{"page":1,"pageSize":100,"totalPages":1,"totalItems":0}}`)
	}))
	defer srv.Close()
	c := testClient(t, srv)

	_, err := c.Dataset("employee").
		Fields("id", "lastChanged").
		Filter("status in ('Active','Inactive')").
		SortBy("lastChanged", true).
		All(context.Background())
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if _, ok := body["fields"]; !ok {
		t.Fatalf("no fields in body: %v", body)
	}
	filter, ok := body["filter"].(string)
	if !ok || filter != "status in ('Active','Inactive')" {
		t.Fatalf("filter = %v, want OData string", body["filter"])
	}
	orderBy, ok := body["orderBy"].(string)
	if !ok || orderBy != "lastChanged desc" {
		t.Fatalf("orderBy = %v, want \"lastChanged desc\"", body["orderBy"])
	}
	// pageSize must be camelCase in the serialized body.
	if _, ok := body["pageSize"]; !ok {
		t.Fatalf("pageSize key missing from body (check camelCase): %v", body)
	}
}
```

- [ ] **Step 2: Run → fail**

Run: `go test ./... -run TestDataset -v`
Expected: FAIL — undefined `Dataset`/`DatasetQuery`.

- [ ] **Step 3: Write `datasets.go`**

```go
package bamboohr

import (
	"context"
	"iter"
	"strings"
)

const defaultPageSize = 100

// DatasetQuery builds a query against a BambooHR v2 dataset (e.g. "employee").
// Use Dataset(name) to construct one.
type DatasetQuery struct {
	c        *Client
	name     string
	fields   []string
	filter   string // raw OData filter expression; multiple Filter calls join with " and "
	orderBy  string // comma-joined "<field> asc|desc" clauses
	pageSize int
}

// Dataset starts a query against the named dataset.
func (c *Client) Dataset(name string) *DatasetQuery {
	return &DatasetQuery{c: c, name: name, pageSize: defaultPageSize}
}

// Fields appends field names to request.
func (q *DatasetQuery) Fields(fields ...string) *DatasetQuery {
	q.fields = append(q.fields, fields...)
	return q
}

// Filter appends an OData filter expression. Multiple calls are joined with " and ".
// Example: Filter("status in ('Active','Inactive')").
func (q *DatasetQuery) Filter(odata string) *DatasetQuery {
	if q.filter == "" {
		q.filter = odata
	} else {
		q.filter = q.filter + " and " + odata
	}
	return q
}

// SortBy appends a sort clause. field must also appear in Fields. Multiple calls
// produce a comma-separated orderBy string (e.g. "lastName asc, lastChanged desc").
func (q *DatasetQuery) SortBy(field string, desc bool) *DatasetQuery {
	direction := "asc"
	if desc {
		direction = "desc"
	}
	clause := field + " " + direction
	if q.orderBy == "" {
		q.orderBy = clause
	} else {
		q.orderBy = q.orderBy + ", " + clause
	}
	return q
}

// PageSize sets the number of records per page (default 100, max 1000).
func (q *DatasetQuery) PageSize(n int) *DatasetQuery {
	if n > 0 {
		q.pageSize = n
	}
	return q
}

type datasetRequest struct {
	Fields   []string `json:"fields"`
	Filter   string   `json:"filter,omitempty"`
	OrderBy  string   `json:"orderBy,omitempty"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"` // camelCase per v2 spec
}

type datasetResponse struct {
	Data []struct {
		Fields Record `json:"fields"`
	} `json:"data"`
	Meta struct {
		Page       int `json:"page"`
		PageSize   int `json:"pageSize"`
		TotalPages int `json:"totalPages"`
		TotalItems int `json:"totalItems"`
	} `json:"meta"`
}

func (q *DatasetQuery) requestBody(page int) datasetRequest {
	return datasetRequest{
		Fields:   q.fields,
		Filter:   q.filter,
		OrderBy:  q.orderBy,
		Page:     page,
		PageSize: q.pageSize,
	}
}

func (q *DatasetQuery) path() string {
	return "api/v2/datasets/" + strings.TrimPrefix(q.name, "/") + "/data"
}

// All fetches every matching record, paging transparently using meta.totalPages.
func (q *DatasetQuery) All(ctx context.Context) ([]Record, error) {
	var all []Record
	for rec, err := range q.Iter(ctx) {
		if err != nil {
			return nil, err
		}
		all = append(all, rec)
	}
	return all, nil
}

// Iter yields records one at a time, fetching pages on demand. Stops when
// page >= meta.totalPages, when data is empty, when the consumer breaks, or
// on the first error (yielded as the second value).
func (q *DatasetQuery) Iter(ctx context.Context) iter.Seq2[Record, error] {
	return func(yield func(Record, error) bool) {
		for page := 1; ; page++ {
			var resp datasetResponse
			if err := q.c.do(ctx, "POST", q.path(), q.requestBody(page), &resp); err != nil {
				yield(nil, err)
				return
			}
			for _, row := range resp.Data {
				if !yield(row.Fields, nil) {
					return
				}
			}
			if len(resp.Data) == 0 || page >= resp.Meta.TotalPages {
				return
			}
		}
	}
}
```

- [ ] **Step 4: Run → pass**

Run: `go test ./... -run TestDataset -v && gofmt -l . && go vet ./...`
Expected: PASS; clean.

- [ ] **Step 5: Commit**

```bash
git add datasets.go datasets_test.go
git commit -m "feat: datasets — v2 API, OData filter, orderBy string, meta.totalPages pagination"
```

---

### Task 8: CLI (`cmd/bamboohr`) for manual smoke

**Files:**
- Create: `cmd/bamboohr/main.go`
- Test: `cmd/bamboohr/main_test.go`

**Interfaces:**
- Consumes: the whole client.
- Produces: a CLI with subcommands `meta-fields`, `employee <id> --fields=...`, `changed --since=<RFC3339>`, `dataset <name> --fields=... [--page-size=N]`. Reads `BAMBOOHR_API_KEY` + `BAMBOOHR_SUBDOMAIN` from env. Prints JSON.

- [ ] **Step 1: Write the failing test** (flag parsing / config building, no network)

`cmd/bamboohr/main_test.go`:
```go
package main

import "testing"

func TestConfigFromEnvMissing(t *testing.T) {
	t.Setenv("BAMBOOHR_API_KEY", "")
	t.Setenv("BAMBOOHR_SUBDOMAIN", "")
	if _, err := configFromEnv(); err == nil {
		t.Fatal("configFromEnv with no env = nil error, want error")
	}
}

func TestConfigFromEnvOK(t *testing.T) {
	t.Setenv("BAMBOOHR_API_KEY", "k")
	t.Setenv("BAMBOOHR_SUBDOMAIN", "acme")
	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("configFromEnv: %v", err)
	}
	if cfg.APIKey != "k" || cfg.Subdomain != "acme" {
		t.Fatalf("cfg = %+v", cfg)
	}
}
```

- [ ] **Step 2: Run → fail**

Run: `go test ./cmd/bamboohr/ -v`
Expected: FAIL — undefined `configFromEnv`.

- [ ] **Step 3: Write `cmd/bamboohr/main.go`**

```go
// Command bamboohr is a thin CLI over the bamboohr client for manual smoke
// testing. Reads BAMBOOHR_API_KEY and BAMBOOHR_SUBDOMAIN from the environment
// (load secrets via: envwith -f ~/.secrets/bamboohr.env -- bamboohr ...).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scjalliance/bamboohr"
)

func configFromEnv() (bamboohr.Config, error) {
	cfg := bamboohr.Config{
		APIKey:    os.Getenv("BAMBOOHR_API_KEY"),
		Subdomain: os.Getenv("BAMBOOHR_SUBDOMAIN"),
	}
	if cfg.APIKey == "" || cfg.Subdomain == "" {
		return cfg, fmt.Errorf("BAMBOOHR_API_KEY and BAMBOOHR_SUBDOMAIN must be set")
	}
	return cfg, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: bamboohr <meta-fields|employee|changed|dataset> [args]")
		os.Exit(2)
	}
	cfg, err := configFromEnv()
	if err != nil {
		fatal(err)
	}
	c, err := bamboohr.New(cfg)
	if err != nil {
		fatal(err)
	}
	ctx := context.Background()

	switch os.Args[1] {
	case "meta-fields":
		out, err := c.Fields(ctx)
		emit(out, err)
	case "employee":
		fs := flag.NewFlagSet("employee", flag.ExitOnError)
		fields := fs.String("fields", "", "comma-separated field list")
		fs.Parse(os.Args[3:])
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("usage: bamboohr employee <id> --fields=..."))
		}
		rec, err := c.GetEmployee(ctx, os.Args[2], splitCSV(*fields)...)
		emit(rec, err)
	case "changed":
		fs := flag.NewFlagSet("changed", flag.ExitOnError)
		since := fs.String("since", "", "RFC3339 timestamp")
		fs.Parse(os.Args[2:])
		ts, perr := time.Parse(time.RFC3339, *since)
		if perr != nil {
			fatal(fmt.Errorf("--since must be RFC3339: %w", perr))
		}
		out, err := c.ChangedSince(ctx, ts)
		emit(out, err)
	case "dataset":
		fs := flag.NewFlagSet("dataset", flag.ExitOnError)
		fields := fs.String("fields", "", "comma-separated field list")
		pageSize := fs.Int("page-size", 100, "page size")
		fs.Parse(os.Args[3:])
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("usage: bamboohr dataset <name> --fields=..."))
		}
		out, err := c.Dataset(os.Args[2]).Fields(splitCSV(*fields)...).PageSize(*pageSize).All(ctx)
		emit(out, err)
	default:
		fatal(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func emit(v any, err error) {
	if err != nil {
		fatal(err)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
```

- [ ] **Step 4: Run → pass + build**

Run: `go test ./cmd/bamboohr/ -v && go build ./... && go vet ./... && gofmt -l .`
Expected: PASS; builds; clean.

- [ ] **Step 5: Commit**

```bash
git add cmd/bamboohr/
git commit -m "feat: cmd/bamboohr CLI for manual smoke testing"
```

---

### Task 9: Write-ready placeholder + README finalize

**Files:**
- Create: `tables.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: nothing new.
- Produces: documented (unimplemented) shape for future per-employee table writes; finalized README quick-start.

- [ ] **Step 1: Write `tables.go`** (doc-only reservation; no behavior, compiles)

```go
package bamboohr

// Future write surface (NOT implemented in v1) — reserved here so the shape is
// agreed and discoverable. The transport (auth, retry, errors) is already
// write-ready; only these endpoints + their tests are deferred to the Assets phase.
//
// Planned API:
//
//	func (c *Client) EmployeeTable(employeeID, table string) *TableRef
//	func (t *TableRef) AddRow(ctx context.Context, row Record) (rowID string, err error)
//	func (t *TableRef) UpdateRow(ctx context.Context, rowID string, row Record) error
//
// against POST /employees/{id}/tables/{table} and
// POST /employees/{id}/tables/{table}/{rowId}. The "Assets" employee table is the
// first intended consumer (recording assigned IT assets per employee).
```

- [ ] **Step 2: Finalize `README.md` quick start** (replace the Quick start section with real, current calls)

```markdown
## Quick start

    c, err := bamboohr.New(bamboohr.Config{APIKey: key, Subdomain: "acme"})
    if err != nil { /* ... */ }

    // One employee:
    rec, _ := c.GetEmployee(ctx, "42", "firstName", "lastName", "jobTitle")
    name := rec.String("firstName")

    // Bulk via the employee dataset (paginates transparently):
    recs, _ := c.Dataset("employee").
        Fields("id", "displayName", "jobTitle", "hireDate").
        SortBy("lastChanged", true).
        All(ctx)

    // Incremental:
    changed, _ := c.ChangedSince(ctx, since)

    // Metadata:
    fields, _ := c.Fields(ctx)

### Dates

Date-only fields are returned as a timezone-free `Date` (not `time.Time`):

    if d, ok := rec.Date("hireDate"); ok { /* d.Year, d.Month, d.Day */ }

The business meaning of a date (start vs end of day, in which timezone) is the
caller's to decide — this client never imposes one.
```

- [ ] **Step 3: Verify whole module + commit**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./... -race`
Expected: all green; `gofmt -l` empty.
```bash
git add tables.go README.md
git commit -m "docs: reserve future table-write surface; finalize README quick start"
```

---

## Self-review notes

- **Spec coverage:** transport/auth/errors/retry (T4), Record+Date+APIError (T3), Config (T2), datasets w/ pagination (T7), employees + changed (T6), meta incl. Tables for future Assets (T5), CLI (T8), write-ready reservation (T9), project setup + CI (T1). All spec sections mapped.
- **Generic/policy-free:** no `customTable####` semantics or name/tz rules anywhere; the only place `customTableNNNN` appears is as a caller-supplied field string in a test, which is correct (the client doesn't interpret it).
- **Known live-verify:** Task 7 Step 0 pins the dataset page-size param casing + envelope against the real tenant before relying on it; iteration logic is independent of that detail.
- **Retry-After** is threaded as a local in `do`'s loop (no globals; concurrency-safe).
