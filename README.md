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

    // Metadata:
    fields, _ := c.Fields(ctx)

### Dates

Date-only fields are returned as a timezone-free `Date` (not `time.Time`):

    if d, ok := rec.Date("hireDate"); ok { /* d.Year, d.Month, d.Day */ }

The business meaning of a date (start vs end of day, in which timezone) is the
caller's to decide — this client never imposes one.

## Docs

- [Design spec](docs/superpowers/specs/2026-06-24-bamboohr-client-design.md)
- [Implementation plan](docs/superpowers/plans/2026-06-24-bamboohr-client.md)
