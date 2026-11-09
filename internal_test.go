package gojev

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestBackoffDelayAppliesJitterWithoutRetryAfter(t *testing.T) {
	policy := DefaultRetryPolicy() // RespectRetryAfter is true
	seen := map[float64]bool{}
	for i := 0; i < 50; i++ {
		d := backoffDelay(0, policy, http.Header{})
		if d < 0.375 || d > 0.625 {
			t.Fatalf("delay %v outside jitter range", d)
		}
		seen[d] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected jittered delays, got a constant %v", seen)
	}
}

func TestBackoffDelayRetryAfter(t *testing.T) {
	policy := DefaultRetryPolicy()
	policy.BackoffJitter = 0

	h := http.Header{}
	h.Set("Retry-After", "3")
	if d := backoffDelay(0, policy, h); d != 3 {
		t.Errorf("expected 3s from Retry-After seconds, got %v", d)
	}

	h.Set("Retry-After", "3600")
	if d := backoffDelay(0, policy, h); d != maxRetryAfter {
		t.Errorf("expected Retry-After capped at %v, got %v", maxRetryAfter, d)
	}

	h.Set("Retry-After", time.Now().Add(30*time.Second).UTC().Format(http.TimeFormat))
	if d := backoffDelay(0, policy, h); d < 15 || d > 30 {
		t.Errorf("expected ~30s from Retry-After HTTP date, got %v", d)
	}

	h.Set("Retry-After", "garbage")
	if d := backoffDelay(0, policy, h); d != policy.BackoffInitial {
		t.Errorf("expected fallback to backoff, got %v", d)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestTransportErrorClassifiesNetTimeout(t *testing.T) {
	var te *TimeoutError
	if err := transportError(context.Background(), timeoutErr{}); !errors.As(err, &te) {
		t.Errorf("expected *TimeoutError for net timeout, got %T", err)
	}
	var ce *ConnectionError
	if err := transportError(context.Background(), errors.New("connection refused")); !errors.As(err, &ce) {
		t.Errorf("expected *ConnectionError, got %T", err)
	}
}

func TestBackoffDelayJitterStaysWithinMax(t *testing.T) {
	policy := DefaultRetryPolicy()
	for i := 0; i < 200; i++ {
		if d := backoffDelay(10, policy, nil); d > policy.BackoffMax {
			t.Fatalf("delay %v exceeds BackoffMax %v", d, policy.BackoffMax)
		}
	}
}

func TestEncodeBodyNullWithExtraBody(t *testing.T) {
	var p *struct{}
	_, err := encodeBody(p, &RequestConfig{ExtraBody: map[string]any{"k": 1}})
	if err == nil {
		t.Fatal("expected an error for a null body with extra body fields")
	}
}

func TestBackoffDelayZeroMaxMeansNoCap(t *testing.T) {
	policy := &RetryPolicy{BackoffInitial: 1}
	if d := backoffDelay(2, policy, nil); d != 4 {
		t.Errorf("expected uncapped 4s delay, got %v", d)
	}
}

func TestAnswerIsComparable(_ *testing.T) {
	_ = map[Answer]struct{}{} // compile-time check: Answer must stay comparable
}

func TestBackoffDelayUncappedStaysRepresentable(t *testing.T) {
	policy := &RetryPolicy{BackoffInitial: 1}
	d := backoffDelay(100, policy, nil)
	if dur := time.Duration(d * float64(time.Second)); dur <= 0 {
		t.Errorf("uncapped delay overflowed time.Duration: %v", dur)
	}
}

func TestRetryAfterMsTakesPrecedence(t *testing.T) {
	policy := DefaultRetryPolicy()
	h := http.Header{}
	h.Set("retry-after-ms", "250")
	h.Set("Retry-After", "5")
	if d := backoffDelay(0, policy, h); d != 0.25 {
		t.Errorf("expected 0.25s from retry-after-ms, got %v", d)
	}
}

func TestBackoffDelayKeepsJitterAtCap(t *testing.T) {
	policy := &RetryPolicy{BackoffInitial: 1, BackoffMax: 5, BackoffJitter: 0.25}
	seen := map[float64]bool{}
	for i := 0; i < 50; i++ {
		d := backoffDelay(10, policy, nil)
		if d > 5 || d < 5*0.75 {
			t.Fatalf("capped delay %v outside [3.75, 5]", d)
		}
		seen[d] = true
	}
	if len(seen) < 2 {
		t.Error("capped delays are not jittered")
	}
}

func TestSleepBackoffFailsFastPastDeadline(t *testing.T) {
	policy := DefaultRetryPolicy()
	h := http.Header{}
	h.Set("Retry-After", "30")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if err := sleepBackoff(ctx, 0, policy, h); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Error("sleepBackoff waited although the delay exceeds the deadline")
	}
}

func TestParseErrorBodySkipsEmptyValues(t *testing.T) {
	for _, body := range []string{
		`{"error":"","message":"boom"}`,
		`{"detail":[],"message":"boom"}`,
	} {
		if msg, _ := parseErrorBody([]byte(body)); msg != "boom" {
			t.Errorf("%s: expected fallback to message, got %q", body, msg)
		}
	}
}

func TestDefaultBudgetAllowsTimedOutAttemptsToRetry(t *testing.T) {
	c, err := NewClient("key")
	if err != nil {
		t.Fatal(err)
	}
	p := c.retryPolicy
	// Every attempt may run to the HTTP timeout, plus the largest backoff between attempts.
	need := time.Duration(p.MaxRetries+1)*c.httpClient.Timeout +
		time.Duration(float64(p.MaxRetries)*p.BackoffMax*float64(time.Second))
	if p.Timeout < need {
		t.Errorf("default call budget %v cannot fit %d timed-out attempts (needs %v)", p.Timeout, p.MaxRetries+1, need)
	}
}
