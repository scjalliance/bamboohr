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
		if r.URL.Query().Get("onlyCurrent") != "true" {
			t.Errorf("onlyCurrent = %q, want true", r.URL.Query().Get("onlyCurrent"))
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

func TestGetEmployeeEscapesID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v1/employees/foo%2Fbar" {
			t.Errorf("EscapedPath = %q, want /api/v1/employees/foo%%2Fbar", r.URL.EscapedPath())
		}
		w.Write([]byte(`{"id":"foo/bar"}`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	rec, err := c.GetEmployee(context.Background(), "foo/bar")
	if err != nil {
		t.Fatalf("GetEmployee: %v", err)
	}
	if rec.String("id") != "foo/bar" {
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

func TestChangedSinceFallsBackToKey(t *testing.T) {
	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/employees/changed" {
			t.Errorf("path = %q", r.URL.Path)
		}
		// Return an entry with omitted id field; the key should be used as fallback
		w.Write([]byte(`{"latest":"2026-06-23T20:21:20+00:00","employees":{
			"99":{"action":"Deleted","lastChanged":"2026-06-20T10:00:00+00:00"}}}`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	changed, err := c.ChangedSince(context.Background(), since)
	if err != nil {
		t.Fatalf("ChangedSince: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("len = %d, want 1", len(changed))
	}
	if changed[0].ID != "99" {
		t.Fatalf("ID = %q, want 99", changed[0].ID)
	}
	if changed[0].Action != "Deleted" {
		t.Fatalf("Action = %q, want Deleted", changed[0].Action)
	}
}

func TestChangedSinceWithType(t *testing.T) {
	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/employees/changed" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("since"); got != since.Format(time.RFC3339) {
			t.Errorf("since = %q", got)
		}
		if got := r.URL.Query().Get("type"); got != "deleted" {
			t.Errorf("type = %q, want deleted", got)
		}
		w.Write([]byte(`{"employees":{}}`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	_, err := c.ChangedSince(context.Background(), since, "deleted")
	if err != nil {
		t.Fatalf("ChangedSince: %v", err)
	}
}
