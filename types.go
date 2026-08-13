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

// TableRow is one row of an employee table (e.g. a single "customTitles" entry).
// ID and EmployeeID are lifted out of the row; every other column lands in Fields,
// keyed by the table's field alias. BambooHR renders table cells as strings (a JSON
// null becomes an absent/nil value), so use Record's typed accessors to read them.
type TableRow struct {
	ID         string // row id, unique within the table
	EmployeeID string // the employee this row belongs to (matches the "eeid" dataset field)
	Fields     Record // field alias → value
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
