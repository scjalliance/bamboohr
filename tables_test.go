package bamboohr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Payloads below mirror the real endpoint SHAPES (numeric id/employeeId, blank and
// null cells, tenant-specific aliases) with synthetic values.

func TestEmployeeTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/employees/7/tables/customTableAlias2" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"id":101,"employeeId":7,"customPersonField":"Ada Lovelace","customExpiryField":"2027-03-01"},
		                 {"id":102,"employeeId":7,"customPersonField":"Grace Hopper","customExpiryField":"2027-03-03"}]`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	rows, err := c.EmployeeTable(context.Background(), "7", "customTableAlias2")
	if err != nil {
		t.Fatalf("EmployeeTable: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len = %d, want 2", len(rows))
	}
	// id/employeeId arrive as JSON numbers and must normalize to strings.
	if rows[0].ID != "101" || rows[0].EmployeeID != "7" {
		t.Fatalf("row[0] id/employeeID = %q/%q", rows[0].ID, rows[0].EmployeeID)
	}
	if got := rows[0].Fields.String("customPersonField"); got != "Ada Lovelace" {
		t.Fatalf("delegate = %q", got)
	}
	if d, ok := rows[1].Fields.Date("customExpiryField"); !ok || d.String() != "2027-03-03" {
		t.Fatalf("row[1] exp = %v ok=%v", d, ok)
	}
	// id/employeeId must not leak into Fields.
	if rows[0].Fields.Has("id") || rows[0].Fields.Has("employeeId") {
		t.Fatalf("Fields = %v (should exclude id/employeeId)", rows[0].Fields)
	}
}

func TestAllEmployeeTables(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/employees/all/tables/customTableAlias" {
			t.Errorf("path = %q", r.URL.Path)
		}
		// Both quirks are real: customDateField can be an empty string, and customFieldA
		// can be JSON null.
		w.Write([]byte(`[{"id":201,"employeeId":8,"customFieldA":"Widget Wrangler","customDateField":""},
		                 {"id":202,"employeeId":8,"customFieldA":"Senior Widget Wrangler","customDateField":"2022-09-22"},
		                 {"id":203,"employeeId":9,"customFieldA":null,"customDateField":"2023-10-23"}]`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	rows, err := c.AllEmployeeTables(context.Background(), "customTableAlias")
	if err != nil {
		t.Fatalf("AllEmployeeTables: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len = %d, want 3", len(rows))
	}
	if got := rows[0].Fields.String("customDateField"); got != "" {
		t.Fatalf("empty date = %q", got)
	}
	if got := rows[2].Fields.String("customFieldA"); got != "" {
		t.Fatalf("null e-sig = %q, want empty", got)
	}

	byEmp := TableRowsByEmployee(rows)
	if len(byEmp["8"]) != 2 || len(byEmp["9"]) != 1 {
		t.Fatalf("grouped = %v", byEmp)
	}
	if byEmp["8"][0].ID != "201" || byEmp["8"][1].ID != "202" {
		t.Fatalf("group order not preserved: %v", byEmp["8"])
	}
}

func TestEmployeeTableEscapesAlias(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	// A tenant alias with a character that must survive the round trip.
	if _, err := c.EmployeeTable(context.Background(), "1", "customAlias'WithQuote"); err != nil {
		t.Fatalf("EmployeeTable: %v", err)
	}
	if want := "/api/v1/employees/1/tables/customAlias%27WithQuote"; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestEmployeeTableRequiresArgs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %q", r.URL.Path)
	}))
	defer srv.Close()
	c := testClient(t, srv)

	if _, err := c.EmployeeTable(context.Background(), "", "customTableAlias"); err == nil {
		t.Fatal("empty employeeID: want error")
	}
	if _, err := c.EmployeeTable(context.Background(), "7", ""); err == nil {
		t.Fatal("empty table: want error")
	}
	if _, err := c.AllEmployeeTables(context.Background(), ""); err == nil {
		t.Fatal("empty table: want error")
	}
}

func TestTableRowsByEmployeeDropsUnattributedRows(t *testing.T) {
	got := TableRowsByEmployee([]TableRow{{ID: "1"}, {ID: "2", EmployeeID: "7"}})
	if len(got) != 1 || len(got["7"]) != 1 {
		t.Fatalf("grouped = %v", got)
	}
}

// TestAddTableRow checks the v1_1 path, the JSON body, and that an empty 200
// response is success.
func TestAddTableRow(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	if err := c.AddTableRow(context.Background(), "7", "customTableAlias", Record{"description": "X1"}); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1_1/employees/7/tables/customTableAlias" || gotBody["description"] != "X1" {
		t.Errorf("got %s %s %v", gotMethod, gotPath, gotBody)
	}
}

// TestAddTableRowRetryPolicy: an add is retried on 429 but not on 503, where
// the row may already have been written.
func TestAddTableRowRetryPolicy(t *testing.T) {
	for _, tc := range []struct {
		status int
		calls  int
	}{{http.StatusTooManyRequests, 2}, {http.StatusServiceUnavailable, 1}} {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.WriteHeader(tc.status)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		c := testClient(t, srv)
		err := c.AddTableRow(context.Background(), "7", "t", Record{"a": "b"})
		srv.Close()
		if calls != tc.calls {
			t.Errorf("status %d: %d calls, want %d (err %v)", tc.status, calls, tc.calls, err)
		}
	}
}

// TestUpdateTableRow checks the row path and that an update retries on 503.
func TestUpdateTableRow(t *testing.T) {
	calls := 0
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotPath = r.URL.Path
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	if err := c.UpdateTableRow(context.Background(), "7", "t", "55", Record{"dateReturned": "2026-10-02"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || gotPath != "/api/v1_1/employees/7/tables/t/55" {
		t.Errorf("calls %d path %s", calls, gotPath)
	}
	if err := c.UpdateTableRow(context.Background(), "7", "t", "", nil); err == nil {
		t.Error("empty rowID accepted")
	}
	if err := c.AddTableRow(context.Background(), "all", "t", nil); err == nil {
		t.Error("employee 'all' accepted for a write")
	}
}
