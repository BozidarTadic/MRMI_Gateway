package delivery

import (
	"context"
	"time"
)

// RetryPolicy defines exponential backoff parameters for envelope forwarding.
// Follows ADR-007: base 1s, multiplier 2, cap 5m, max 10 attempts.
type RetryPolicy struct {
	BaseDelay   time.Duration
	Multiplier  float64
	Cap         time.Duration
	MaxAttempts int
}

// DefaultRetryPolicy returns the ADR-007 defaults.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		BaseDelay:   time.Second,
		Multiplier:  2.0,
		Cap:         5 * time.Minute,
		MaxAttempts: 10,
	}
}

// SendWithRetry calls send() repeatedly with exponential backoff until it
// succeeds, the context is cancelled, or MaxAttempts is exhausted.
//
// On exhaustion the entry is written to dlq (if non-nil) and the last error
// is returned.
func SendWithRetry(ctx context.Context, send func() error, policy RetryPolicy, dlq *DLQ, entry DLQEntry) error {
	delay := policy.BaseDelay
	var lastErr error

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		lastErr = send()
		if lastErr == nil {
			return nil
		}

		entry.Attempts = attempt
		entry.LastErr = lastErr
		entry.LastAttemptUnix = time.Now().Unix()

		if attempt == policy.MaxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		delay = time.Duration(float64(delay) * policy.Multiplier)
		if delay > policy.Cap {
			delay = policy.Cap
		}
	}

	if dlq != nil {
		dlq.Append(entry)
	}
	return lastErr
}
