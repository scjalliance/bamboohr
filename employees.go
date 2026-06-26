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
// If changeType is provided (variadic, max 1 value), it sets an optional BambooHR "type" query param
// (enum: "inserted", "updated", "deleted", "all"); the API interprets case-insensitive values.
// Only the first value in changeType is used; additional values are ignored.
func (c *Client) ChangedSince(ctx context.Context, since time.Time, changeType ...string) ([]ChangedEmployee, error) {
	q := url.Values{}
	q.Set("since", since.Format(time.RFC3339))
	if len(changeType) > 0 && changeType[0] != "" {
		q.Set("type", changeType[0])
	}
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
