package schema

import (
	"testing"
	"time"

	"MRMI_Gateway/internal/config"
)

func TestIsInWindow_InsideWindow(t *testing.T) {
	w := config.CutoffWindow{Open: "08:00", Close: "17:00", TZ: "UTC"}
	// 12:30 UTC is inside 08:00–17:00
	ts := time.Date(2026, 6, 2, 12, 30, 0, 0, time.UTC)

	ok, err := IsInWindow(w, ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected inside window")
	}
}

func TestIsInWindow_BeforeOpen(t *testing.T) {
	w := config.CutoffWindow{Open: "08:00", Close: "17:00", TZ: "UTC"}
	ts := time.Date(2026, 6, 2, 7, 59, 0, 0, time.UTC)

	ok, err := IsInWindow(w, ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected outside window (before open)")
	}
}

func TestIsInWindow_AfterClose(t *testing.T) {
	w := config.CutoffWindow{Open: "08:00", Close: "17:00", TZ: "UTC"}
	ts := time.Date(2026, 6, 2, 17, 0, 0, 0, time.UTC)

	ok, err := IsInWindow(w, ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected outside window (at close boundary — exclusive)")
	}
}

func TestIsInWindow_MidnightWrapping_Inside(t *testing.T) {
	// Window 22:00–06:00 crosses midnight
	w := config.CutoffWindow{Open: "22:00", Close: "06:00", TZ: "UTC"}
	ts := time.Date(2026, 6, 2, 23, 30, 0, 0, time.UTC)

	ok, err := IsInWindow(w, ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected inside midnight-wrapping window")
	}
}

func TestIsInWindow_MidnightWrapping_EarlyMorning(t *testing.T) {
	w := config.CutoffWindow{Open: "22:00", Close: "06:00", TZ: "UTC"}
	ts := time.Date(2026, 6, 2, 3, 0, 0, 0, time.UTC)

	ok, err := IsInWindow(w, ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected inside midnight-wrapping window (early morning)")
	}
}

func TestIsInWindow_MidnightWrapping_Outside(t *testing.T) {
	w := config.CutoffWindow{Open: "22:00", Close: "06:00", TZ: "UTC"}
	ts := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	ok, err := IsInWindow(w, ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected outside midnight-wrapping window")
	}
}

func TestIsInWindow_InvalidTimezone(t *testing.T) {
	w := config.CutoffWindow{Open: "08:00", Close: "17:00", TZ: "Not/AReal/Zone"}
	_, err := IsInWindow(w, time.Now())
	if err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}

func TestIsInWindow_InvalidTimeFormat(t *testing.T) {
	w := config.CutoffWindow{Open: "8am", Close: "17:00", TZ: "UTC"}
	_, err := IsInWindow(w, time.Now())
	if err == nil {
		t.Fatal("expected error for invalid open time format")
	}
}

func TestIsInWindow_ExactOpenBoundaryIsInside(t *testing.T) {
	w := config.CutoffWindow{Open: "08:00", Close: "17:00", TZ: "UTC"}
	ts := time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC)

	ok, err := IsInWindow(w, ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected at-open boundary to be inside window")
	}
}

func TestParseTOD_Valid(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"00:00", 0},
		{"08:00", 480},
		{"17:30", 1050},
		{"23:59", 1439},
	}
	for _, c := range cases {
		got, err := parseTOD(c.s)
		if err != nil {
			t.Fatalf("parseTOD(%q): unexpected error: %v", c.s, err)
		}
		if got != c.want {
			t.Fatalf("parseTOD(%q): want %d, got %d", c.s, c.want, got)
		}
	}
}

func TestParseTOD_Invalid(t *testing.T) {
	for _, s := range []string{"8:00", "25:00", "08:60", "abc", ""} {
		_, err := parseTOD(s)
		if err == nil {
			t.Fatalf("parseTOD(%q): expected error", s)
		}
	}
}
