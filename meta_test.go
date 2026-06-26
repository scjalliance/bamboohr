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
