package schema

import (
	"fmt"
	"time"

	"MRMI_Gateway/internal/config"
)

// Iso20022 is the schema_type value for ISO 20022 financial messages (ADR-016).
const Iso20022 = "iso20022"

// IsInWindow reports whether t falls within the processing window w.
// Open and Close are parsed as 24-hour "HH:MM" strings local to w.TZ.
// Windows that cross midnight (e.g. Open="22:00", Close="06:00") are handled correctly.
// Returns an error if w is misconfigured (invalid timezone or time format).
func IsInWindow(w config.CutoffWindow, t time.Time) (bool, error) {
	loc, err := time.LoadLocation(w.TZ)
	if err != nil {
		return false, fmt.Errorf("iso20022: cutoff window: invalid timezone %q: %w", w.TZ, err)
	}

	open, err := parseTOD(w.Open)
	if err != nil {
		return false, fmt.Errorf("iso20022: cutoff window open: %w", err)
	}
	close_, err := parseTOD(w.Close)
	if err != nil {
		return false, fmt.Errorf("iso20022: cutoff window close: %w", err)
	}

	now := timeOfDay(t.In(loc))

	if open <= close_ {
		// Normal window: e.g. 08:00–17:00
		return now >= open && now < close_, nil
	}
	// Wrapping window: e.g. 22:00–06:00 (crosses midnight)
	return now >= open || now < close_, nil
}

// NextOpen returns the next time window w opens at or after t.
// It is intended to be called when t is already known to be outside the window.
func NextOpen(w config.CutoffWindow, t time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(w.TZ)
	if err != nil {
		return time.Time{}, fmt.Errorf("iso20022: cutoff window: invalid timezone %q: %w", w.TZ, err)
	}
	open, err := parseTOD(w.Open)
	if err != nil {
		return time.Time{}, fmt.Errorf("iso20022: cutoff window open: %w", err)
	}
	close_, err := parseTOD(w.Close)
	if err != nil {
		return time.Time{}, fmt.Errorf("iso20022: cutoff window close: %w", err)
	}

	now := t.In(loc)
	nowTOD := timeOfDay(now)
	openToday := time.Date(now.Year(), now.Month(), now.Day(), open/60, open%60, 0, 0, loc)

	if open <= close_ {
		// Normal window (e.g. 08:00-17:00): outside means before open or at/after close.
		if nowTOD < open {
			return openToday, nil // opens later today
		}
		return openToday.Add(24 * time.Hour), nil // opens tomorrow
	}
	// Midnight-wrapping window (e.g. 22:00-06:00): outside means [close_, open).
	return openToday, nil // opens later today at open hour
}

// parseTOD parses "HH:MM" (zero-padded 24-hour) and returns minutes since midnight.
func parseTOD(s string) (int, error) {
	if len(s) != 5 || s[2] != ':' {
		return 0, fmt.Errorf("expected HH:MM, got %q", s)
	}
	var h, m int
	if n, _ := fmt.Sscanf(s, "%d:%d", &h, &m); n != 2 {
		return 0, fmt.Errorf("expected HH:MM, got %q", s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("time %q out of range (hour 0-23, minute 0-59)", s)
	}
	return h*60 + m, nil
}

// timeOfDay returns minutes since midnight for t.
func timeOfDay(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}
