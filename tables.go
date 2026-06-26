package bamboohr

// Future write surface (NOT implemented in v1) — reserved here so the shape is
// agreed and discoverable. The transport (auth, retry, errors) is already
// write-ready; only these endpoints + their tests are deferred to the Assets phase.
//
// Planned API:
//
//	func (c *Client) EmployeeTable(employeeID, table string) *TableRef
//	func (t *TableRef) AddRow(ctx context.Context, row Record) (rowID string, err error)
//	func (t *TableRef) UpdateRow(ctx context.Context, rowID string, row Record) error
//
// against POST /api/v1/employees/{id}/tables/{table} (create) and
// POST /api/v1/employees/{id}/tables/{table}/{rowId} (update). The "Assets"
// employee table is the first intended consumer (recording assigned IT assets
// per employee).
