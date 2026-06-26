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
// Note: BambooHR does NOT allow mixing "and" and "or" operators in a single expression.
// To express OR conditions, pass the entire OR expression in a single Filter() call.
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
			// Stop when data is empty (handles totalPages=0 or end of data) or page >= totalPages.
			// The empty-data guard is the primary terminator, preventing any infinite loop.
			if len(resp.Data) == 0 || page >= resp.Meta.TotalPages {
				return
			}
		}
	}
}
