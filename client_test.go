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
	if err := c.do(context.Background(), http.MethodGet, "api/v1/ping", nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("key:x"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
	if gotAccept != "application/json" {
		t.Fatalf("Accept = %q", gotAccept)
	}
	if gotPath != "/api/v1/ping" {
		t.Fatalf("path = %q, want /api/v1/ping", gotPath)
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
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := testClient(t, srv)

	err := c.do(context.Background(), http.MethodGet, "x", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 {
		t.Fatalf("err = %v, want *APIError 429 after retries", err)
	}
	if calls != 4 {
		t.Fatalf("calls = %d, want 4 (1 attempt + 3 retries)", calls)
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
