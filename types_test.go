package bamboohr

import (
	"testing"
	"time"
)

func TestRecordAccessors(t *testing.T) {
	r := Record{
		"firstName": "Jane",
		"isActive":  "true",
		"count":     float64(3), // JSON numbers decode to float64
		"hireDate":  "2021-12-13",
		"changed":   "2026-06-23T20:21:20+00:00",
		"empty":     "",
	}
	if r.String("firstName") != "Jane" {
		t.Fatalf("String = %q", r.String("firstName"))
	}
	if r.String("count") != "3" {
		t.Fatalf("String(count) = %q, want 3", r.String("count"))
	}
	if !r.Bool("isActive") {
		t.Fatal("Bool(isActive) = false")
	}
	if r.Bool("missing") {
		t.Fatal("Bool(missing) = true, want false")
	}
	if !r.Has("empty") || r.Has("nope") {
		t.Fatal("Has wrong")
	}
	d, ok := r.Date("hireDate")
	if !ok || d.Year != 2021 || d.Month != time.December || d.Day != 13 {
		t.Fatalf("Date(hireDate) = %+v ok=%v", d, ok)
	}
	if _, ok := r.Date("changed"); ok {
		t.Fatal("Date should not parse a full timestamp as a civil date")
	}
	ts, ok := r.Time("changed")
	if !ok || ts.Year() != 2026 || ts.Hour() != 20 {
		t.Fatalf("Time(changed) = %v ok=%v", ts, ok)
	}
}

func TestDateString(t *testing.T) {
	if got := (Date{2021, time.December, 13}).String(); got != "2021-12-13" {
		t.Fatalf("Date.String = %q", got)
	}
}

func TestParseDateRejectsTimestamp(t *testing.T) {
	if _, ok := ParseDate("2026-06-23T20:21:20+00:00"); ok {
		t.Fatal("ParseDate accepted a timestamp")
	}
	if _, ok := ParseDate(""); ok {
		t.Fatal("ParseDate accepted empty")
	}
	if d, ok := ParseDate("2021-12-13"); !ok || d.Day != 13 {
		t.Fatalf("ParseDate(date) = %+v ok=%v", d, ok)
	}
}

func TestAPIError(t *testing.T) {
	e := &APIError{Status: 403, Message: "Forbidden", BambooHRMessage: "no field access"}
	if e.Error() == "" {
		t.Fatal("APIError.Error empty")
	}
}
