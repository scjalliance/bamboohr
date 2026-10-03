package bamboohr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// EmployeeTable reads every row of one employee table (e.g. "customTableAlias") for a
// single employee, via GET /api/v1/employees/{id}/tables/{table}.
func (c *Client) EmployeeTable(ctx context.Context, employeeID, table string) ([]TableRow, error) {
	if employeeID == "" {
		return nil, fmt.Errorf("bamboohr: employeeID is required")
	}
	return c.tableRows(ctx, employeeID, table)
}

// AllEmployeeTables reads one employee table for EVERY employee in a single request,
// via GET /api/v1/employees/all/tables/{table}. Each row carries its own EmployeeID,
// so callers group by that (see TableRowsByEmployee). This is the bulk form — prefer
// it over N calls to EmployeeTable when mirroring a whole population.
func (c *Client) AllEmployeeTables(ctx context.Context, table string) ([]TableRow, error) {
	return c.tableRows(ctx, "all", table)
}

func (c *Client) tableRows(ctx context.Context, employeeID, table string) ([]TableRow, error) {
	path, err := tablePath("v1", employeeID, table, "")
	if err != nil {
		return nil, err
	}

	var raw []map[string]json.RawMessage
	if err := c.get(ctx, path, nil, &raw); err != nil {
		return nil, err
	}
	rows := make([]TableRow, 0, len(raw))
	for _, r := range raw {
		row := TableRow{Fields: Record{}}
		for k, v := range r {
			switch k {
			case "id":
				row.ID = jsonScalarString(v)
			case "employeeId":
				row.EmployeeID = jsonScalarString(v)
			default:
				var val any
				if err := json.Unmarshal(v, &val); err != nil {
					continue
				}
				row.Fields[k] = val
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// TableRowsByEmployee groups rows by EmployeeID, preserving the server's row order
// within each employee. Rows with an empty EmployeeID are dropped.
func TableRowsByEmployee(rows []TableRow) map[string][]TableRow {
	out := make(map[string][]TableRow)
	for _, r := range rows {
		if r.EmployeeID == "" {
			continue
		}
		out[r.EmployeeID] = append(out[r.EmployeeID], r)
	}
	return out
}

// jsonScalarString renders a JSON scalar (string or number) as a string,
// trimming surrounding quotes from string-encoded values. JSON null becomes "".
func jsonScalarString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "null" {
		return ""
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			return str
		}
	}
	return s
}

// AddTableRow adds a row to one employee table, via
// POST /api/v1_1/employees/{id}/tables/{table}. BambooHR returns no body, so the
// new row's id is not known; re-read the table (EmployeeTable) and find the row
// by its content. This is not idempotent: it is retried only on 429 and on
// failures before the request was sent. A failure that may have been applied
// (a network error after sending, or a 503) wraps ErrWriteOutcomeUnknown.
func (c *Client) AddTableRow(ctx context.Context, employeeID, table string, row Record) error {
	if len(row) == 0 {
		return fmt.Errorf("bamboohr: row has no fields")
	}
	if err := singleEmployee(employeeID); err != nil {
		return err
	}
	path, err := tablePath("v1_1", employeeID, table, "")
	if err != nil {
		return err
	}
	return c.doWith(ctx, http.MethodPost, path, row, nil, true)
}

// UpdateTableRow changes the given fields of one existing row in place, via
// POST /api/v1_1/employees/{id}/tables/{table}/{rowId}. Fields not in row keep
// their values. Repeating it is harmless, so it retries like a read.
func (c *Client) UpdateTableRow(ctx context.Context, employeeID, table, rowID string, row Record) error {
	if rowID == "" {
		return fmt.Errorf("bamboohr: rowID is required")
	}
	if len(row) == 0 {
		return fmt.Errorf("bamboohr: row has no fields")
	}
	if err := singleEmployee(employeeID); err != nil {
		return err
	}
	path, err := tablePath("v1_1", employeeID, table, rowID)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, path, row, nil)
}

// singleEmployee rejects the bulk pseudo-id "all" for writes.
func singleEmployee(employeeID string) error {
	if strings.EqualFold(strings.TrimSpace(employeeID), "all") {
		return fmt.Errorf("bamboohr: a single employeeID is required")
	}
	return nil
}

// tablePath builds an employee-table path for an API version ("v1" for reads,
// "v1_1" for writes; the v1 writes were deprecated on 2026-07-08). Table
// aliases are tenant-defined and may need escaping. "." and ".." are refused
// as path segments: a proxy that normalizes them could turn an update URL
// into the add URL.
func tablePath(version, employeeID, table, rowID string) (string, error) {
	if employeeID == "" {
		return "", fmt.Errorf("bamboohr: employeeID is required")
	}
	if table == "" {
		return "", fmt.Errorf("bamboohr: table is required")
	}
	for _, seg := range []string{employeeID, table, rowID} {
		if seg == "." || seg == ".." {
			return "", fmt.Errorf("bamboohr: invalid path segment %q", seg)
		}
	}
	p := "api/" + version + "/employees/" + url.PathEscape(employeeID) + "/tables/" + url.PathEscape(table)
	if rowID != "" {
		p += "/" + url.PathEscape(rowID)
	}
	return p, nil
}
