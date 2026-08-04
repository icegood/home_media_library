package scheduler

import (
	"testing"
	"time"
)

func TestParseCronAcceptsValidExpressions(t *testing.T) {
	valid := []string{
		"0 3 * * *",
		"*/15 * * * *",
		"0 0 1 1 *",
		"30 2 * * 1-5",
		"0,30 * * * *",
		"0 9 * * 7",
		"0 0 * * 0",
	}
	for _, expr := range valid {
		if _, err := ParseCron(expr); err != nil {
			t.Errorf("ParseCron(%q) should succeed: %v", expr, err)
		}
	}
}

func TestParseCronRejectsInvalidExpressions(t *testing.T) {
	invalid := []string{
		"",
		"0 3 * *",
		"60 * * * *",
		"* 24 * * *",
		"0 0 32 * *",
		"0 0 * 13 *",
		"5-2 * * * *",
		"*/0 * * * *",
		"a b c d e",
	}
	for _, expr := range invalid {
		if _, err := ParseCron(expr); err == nil {
			t.Errorf("ParseCron(%q) should fail", expr)
		}
	}
}

func TestCronNext(t *testing.T) {
	base := time.Date(2026, time.August, 6, 10, 30, 0, 0, time.UTC)

	next, err := Next("*/15 * * * *", base)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.August, 6, 10, 45, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("Next(*/15): got %v want %v", next, want)
	}

	next, err = Next("0 3 * * *", base)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2026, time.August, 7, 3, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("Next(0 3): got %v want %v", next, want)
	}

	// day-of-week restricted: next Friday (2026-08-07 is a Friday).
	next, err = Next("0 12 * * 5", base)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("Next(0 12 * * 5): got %v want %v", next, want)
	}

	// 7 is treated as Sunday.
	next, err = Next("0 9 * * 7", base)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2026, time.August, 9, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("Next(0 9 * * 7): got %v want %v", next, want)
	}

	// Both day fields restricted use OR semantics: 15th or Monday.
	next, err = Next("0 0 15 * 1", base)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("Next(0 0 15 * 1): got %v want %v", next, want)
	}
}
