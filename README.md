# bamboohr

Generic Go client for the [BambooHR API](https://documentation.bamboohr.com/reference).
Policy-free: you name the fields, you interpret the values. Built for SCJ's
identity-sync; suitable for general use.

## Install

    go get github.com/scjalliance/bamboohr

## Quick start

    c, err := bamboohr.New(bamboohr.Config{APIKey: key, Subdomain: "acme"})
    if err != nil { /* ... */ }

    // One employee (errors are typed — *bamboohr.APIError carries the HTTP status):
    rec, err := c.GetEmployee(ctx, "42", "firstName", "lastName", "jobTitle")
    if err != nil {
        log.Fatalf("get employee: %v", err)
    }
    name := rec.String("firstName")

    // Bulk via the employee dataset (paginates transparently):
    recs, _ := c.Dataset("employee").
        Fields("id", "displayName", "jobTitle", "hireDate").
        SortBy("lastChanged", true).
        All(ctx)

    // Incremental:
    changed, _ := c.ChangedSince(ctx, since)
    // Table edits (employment status, job info, custom tables) are reported
    // per table:
    statusChanged, _ := c.ChangedTableSince(ctx, "employmentStatus", since)

    // Metadata:
    fields, _ := c.Fields(ctx)
    tables, _ := c.Tables(ctx)

### Employee tables

Employee *tables* hold many rows per person (job-info history, assets, tenant-defined
custom tables). They are NOT reachable through the dataset API — only through
`/api/v1/employees/{id|all}/tables/{alias}`:

    // One person's rows:
    rows, _ := c.EmployeeTable(ctx, "42", "customTableAlias")

    // Every person's rows, in ONE request — prefer this for a whole population:
    all, _ := c.AllEmployeeTables(ctx, "customTableAlias")
    byEmployee := bamboohr.TableRowsByEmployee(all)

`ID` and `EmployeeID` are lifted out of each row; every other column lands in
`Fields`, keyed by the table's field alias, and reads through the same typed
accessors as a dataset record:

    for _, r := range rows {
        title := r.Fields.String("customFieldA")
        if d, ok := r.Fields.Date("customDateField"); ok { /* ... */ }
    }

Writes use the v1_1 endpoints (the v1 ones were deprecated on 2026-07-08):

    // Add a row. BambooHR returns no body, so the new row's id is not known.
    // Write a value unique to this add (here a request id) so a re-read can
    // find exactly this row, and stop on any read error.
    marker := "req-7f3a"
    err := c.AddTableRow(ctx, "42", "customTableAlias",
        bamboohr.Record{"customFieldA": "x", "customNote": marker})
    if err != nil && !errors.Is(err, bamboohr.ErrWriteOutcomeUnknown) {
        return err // the add was not applied
    }
    // On success or an unknown outcome, re-read to find the row.
    rows, err := c.EmployeeTable(ctx, "42", "customTableAlias")
    if err != nil {
        return err // still unresolved: do not add again or update anything
    }
    var matches []string
    for _, r := range rows {
        if r.Fields.String("customNote") == marker {
            matches = append(matches, r.ID)
        }
    }
    if len(matches) != 1 {
        return fmt.Errorf("add unresolved: %d rows carry the marker", len(matches))
    }
    rowID := matches[0]

    // Change some fields of one row in place. A Date marshals as "YYYY-MM-DD".
    err = c.UpdateTableRow(ctx, "42", "customTableAlias", rowID,
        bamboohr.Record{"customDateField": bamboohr.Date{Year: 2026, Month: 10, Day: 2}})

An add is not idempotent, so it is retried only on 429. Any transport error or
5xx returns an error wrapping `ErrWriteOutcomeUnknown`. Re-read the table
before adding again. A row whose fields match the add does not prove the add
succeeded, since an identical row may already have existed. Write a value
unique to the add and find the row by it. A diff of row ids against a
snapshot taken before the add is weaker, because another writer can add a
row in between. If the re-read fails or stays ambiguous, leave the outcome
unresolved. Updates retry like reads.

### Dates

Date-only fields are returned as a timezone-free `Date` (not `time.Time`):

    if d, ok := rec.Date("hireDate"); ok { /* d.Year, d.Month, d.Day */ }

The business meaning of a date (start vs end of day, in which timezone) is the
caller's to decide — this client never imposes one.

## Docs

- [Design spec](docs/superpowers/specs/2026-06-24-bamboohr-client-design.md)
- [Implementation plan](docs/superpowers/plans/2026-06-24-bamboohr-client.md)
