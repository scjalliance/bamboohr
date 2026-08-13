// Command bamboohr is a thin CLI over the bamboohr client for manual smoke
// testing. Reads BAMBOOHR_API_KEY and BAMBOOHR_SUBDOMAIN from the environment
// (load secrets via: envwith -f ~/.secrets/bamboohr.env -- bamboohr ...).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scjalliance/bamboohr"
)

func configFromEnv() (bamboohr.Config, error) {
	cfg := bamboohr.Config{
		APIKey:    os.Getenv("BAMBOOHR_API_KEY"),
		Subdomain: os.Getenv("BAMBOOHR_SUBDOMAIN"),
	}
	if cfg.APIKey == "" || cfg.Subdomain == "" {
		return cfg, fmt.Errorf("BAMBOOHR_API_KEY and BAMBOOHR_SUBDOMAIN must be set")
	}
	return cfg, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: bamboohr <meta-fields|meta-tables|employee|changed|dataset|table> [args]")
		os.Exit(2)
	}
	cfg, err := configFromEnv()
	if err != nil {
		fatal(err)
	}
	c, err := bamboohr.New(cfg)
	if err != nil {
		fatal(err)
	}
	ctx := context.Background()

	switch os.Args[1] {
	case "meta-fields":
		out, err := c.Fields(ctx)
		emit(out, err)
	case "meta-tables":
		out, err := c.Tables(ctx)
		emit(out, err)
	case "table":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("usage: bamboohr table <alias> [--employee=<id>]"))
		}
		fs := flag.NewFlagSet("table", flag.ExitOnError)
		employee := fs.String("employee", "", "employee id (default: every employee)")
		fs.Parse(os.Args[3:])
		if *employee != "" {
			out, err := c.EmployeeTable(ctx, *employee, os.Args[2])
			emit(out, err)
			break
		}
		out, err := c.AllEmployeeTables(ctx, os.Args[2])
		emit(out, err)
	case "employee":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("usage: bamboohr employee <id> --fields=..."))
		}
		fs := flag.NewFlagSet("employee", flag.ExitOnError)
		fields := fs.String("fields", "", "comma-separated field list")
		fs.Parse(os.Args[3:])
		rec, err := c.GetEmployee(ctx, os.Args[2], splitCSV(*fields)...)
		emit(rec, err)
	case "changed":
		fs := flag.NewFlagSet("changed", flag.ExitOnError)
		since := fs.String("since", "", "RFC3339 timestamp")
		fs.Parse(os.Args[2:])
		ts, perr := time.Parse(time.RFC3339, *since)
		if perr != nil {
			fatal(fmt.Errorf("--since must be RFC3339: %w", perr))
		}
		out, err := c.ChangedSince(ctx, ts)
		emit(out, err)
	case "dataset":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("usage: bamboohr dataset <name> --fields=..."))
		}
		fs := flag.NewFlagSet("dataset", flag.ExitOnError)
		fields := fs.String("fields", "", "comma-separated field list")
		pageSize := fs.Int("page-size", 100, "page size")
		fs.Parse(os.Args[3:])
		out, err := c.Dataset(os.Args[2]).Fields(splitCSV(*fields)...).PageSize(*pageSize).All(ctx)
		emit(out, err)
	default:
		fatal(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func emit(v any, err error) {
	if err != nil {
		fatal(err)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
