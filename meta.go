package bamboohr

import (
	"context"
	"encoding/json"
)

// Fields returns every field available to the API key.
func (c *Client) Fields(ctx context.Context) ([]Field, error) {
	// BambooHR field IDs may be numeric or string; decode loosely then normalize.
	var raw []struct {
		ID    json.RawMessage `json:"id"`
		Name  string          `json:"name"`
		Type  string          `json:"type"`
		Alias string          `json:"alias"`
	}
	if err := c.get(ctx, "api/v1/meta/fields", nil, &raw); err != nil {
		return nil, err
	}
	fields := make([]Field, 0, len(raw))
	for _, f := range raw {
		fields = append(fields, Field{ID: jsonScalarString(f.ID), Name: f.Name, Type: f.Type, Alias: f.Alias})
	}
	return fields, nil
}

// Tables returns the employee table definitions (e.g. the Assets table).
// Per spec the response is an array of { alias, fields: [...] }; name may be absent.
// Alias is the stable key; Name is informational only.
func (c *Client) Tables(ctx context.Context) ([]Table, error) {
	var raw []struct {
		Name  string `json:"name"`
		Alias string `json:"alias"`
	}
	if err := c.get(ctx, "api/v1/meta/tables", nil, &raw); err != nil {
		return nil, err
	}
	tables := make([]Table, 0, len(raw))
	for _, t := range raw {
		tables = append(tables, Table{Name: t.Name, Alias: t.Alias})
	}
	return tables, nil
}
