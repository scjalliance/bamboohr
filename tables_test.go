package bamboohr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Payloads below mirror the real endpoint SHAPES (numeric id/employeeId, blank and
// null cells, tenant-specific aliases) with synthetic values.

func TestEmployeeTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/employees/382/tables/customEmailDelegation" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"id":5972,"employeeId":382,"customPrimaryDelegate":"Ada Lovelace","customDelegationExpDate":"2027-03-01"},
		                 {"id":5973,"employeeId":382,"customPrimaryDelegate":"Grace Hopper","customDelegationExpDate":"2027-03-03"}]`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	rows, err := c.EmployeeTable(context.Background(), "382", "customEmailDelegation")
	if err != nil {
		t.Fatalf("EmployeeTable: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len = %d, want 2", len(rows))
	}
	// id/employeeId arrive as JSON numbers and must normalize to strings.
	if rows[0].ID != "5972" || rows[0].EmployeeID != "382" {
		t.Fatalf("row[0] id/employeeID = %q/%q", rows[0].ID, rows[0].EmployeeID)
	}
	if got := rows[0].Fields.String("customPrimaryDelegate"); got != "Ada Lovelace" {
		t.Fatalf("delegate = %q", got)
	}
	if d, ok := rows[1].Fields.Date("customDelegationExpDate"); !ok || d.String() != "2027-03-03" {
		t.Fatalf("row[1] exp = %v ok=%v", d, ok)
	}
	// id/employeeId must not leak into Fields.
	if rows[0].Fields.Has("id") || rows[0].Fields.Has("employeeId") {
		t.Fatalf("Fields = %v (should exclude id/employeeId)", rows[0].Fields)
	}
}

func TestAllEmployeeTables(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/employees/all/tables/customTitles" {
			t.Errorf("path = %q", r.URL.Path)
		}
		// Both quirks are real: customDate1 can be an empty string, and customE-Sig
		// can be JSON null.
		w.Write([]byte(`[{"id":1468,"employeeId":111,"customE-Sig":"Widget Wrangler","customDate1":""},
		                 {"id":2599,"employeeId":111,"customE-Sig":"Senior Widget Wrangler","customDate1":"2022-09-22"},
		                 {"id":3779,"employeeId":324,"customE-Sig":null,"customDate1":"2023-10-23"}]`))
	}))
	defer srv.Close()
	c := testClient(t, srv)

	rows, err := c.AllEmployeeTables(context.Background(), "customTitles")
	if err != nil {
		t.Fatalf("AllEmployeeTables: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len = %d, want 3", len(rows))
	}
	if got := rows[0].Fields.String("customDate1"); got != "" {
		t.Fatalf("empty date = %q", got)
	}
	if got := rows[2].Fields.String("customE-Sig"); got != "" {
		t.Fatalf("null e-sig = %q, want empty", got)
	}

	byEmp := TableRowsByEmployee(rows)
	if len(byEmp["111"]) != 2 || len(byEmp["324"]) != 1 {
		t.Fatalf("grouped = %v", byEmp)
	}
	if byEmp["111"][0].ID != "1468" || byEmp["111"][1].ID != "2599" {
		t.Fatalf("group order not preserved: %v", byEmp["111"])
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
	if _, err := c.EmployeeTable(context.Background(), "1", "customDriver'sLicenseInformation"); err != nil {
		t.Fatalf("EmployeeTable: %v", err)
	}
	if want := "/api/v1/employees/1/tables/customDriver%27sLicenseInformation"; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestEmployeeTableRequiresArgs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %q", r.URL.Path)
	}))
	defer srv.Close()
	c := testClient(t, srv)

	if _, err := c.EmployeeTable(context.Background(), "", "customTitles"); err == nil {
		t.Fatal("empty employeeID: want error")
	}
	if _, err := c.EmployeeTable(context.Background(), "382", ""); err == nil {
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
