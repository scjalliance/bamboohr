package bamboohr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
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
	if calls != 2 {
		t.Fatalf("server calls = %d, want 2 (no over-fetch past break)", calls)
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
	// First request must start at page 1.
	page, ok := body["page"].(float64)
	if !ok || int(page) != 1 {
		t.Fatalf("page = %v, want 1", body["page"])
	}
}
