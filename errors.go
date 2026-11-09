package gojev

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// APIError represents an error returned by the API.
type APIError struct {
	StatusCode int
	Body       []byte
	Header     http.Header
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("API error (status %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("API error (status %d)", e.StatusCode)
}

// BadRequestError represents a 400 Bad Request response.
type BadRequestError struct{ APIError }

func (e *BadRequestError) Unwrap() error { return &e.APIError }

// AuthenticationError represents a 401 Unauthorized response.
type AuthenticationError struct{ APIError }

func (e *AuthenticationError) Unwrap() error { return &e.APIError }

// PermissionDeniedError represents a 403 Forbidden response.
type PermissionDeniedError struct{ APIError }

func (e *PermissionDeniedError) Unwrap() error { return &e.APIError }

// NotFoundError represents a 404 Not Found response.
type NotFoundError struct{ APIError }

func (e *NotFoundError) Unwrap() error { return &e.APIError }

// RequestTooLargeError represents a 413 response: the request body exceeds 32 MiB.
type RequestTooLargeError struct{ APIError }

func (e *RequestTooLargeError) Unwrap() error { return &e.APIError }

// UnsupportedMediaTypeError represents a 415 response: an unsupported Content-Encoding.
type UnsupportedMediaTypeError struct{ APIError }

func (e *UnsupportedMediaTypeError) Unwrap() error { return &e.APIError }

// ValidationError is one entry of a 422 response's "detail" list.
type ValidationError struct {
	Loc   []any          `json:"loc"`
	Msg   string         `json:"msg"`
	Type  string         `json:"type"`
	Input any            `json:"input,omitempty"`
	Ctx   map[string]any `json:"ctx,omitempty"`
}

func (v ValidationError) String() string {
	if len(v.Loc) == 0 {
		return v.Msg
	}
	parts := make([]string, len(v.Loc))
	for i, l := range v.Loc {
		parts[i] = fmt.Sprint(l)
	}
	return strings.Join(parts, ".") + ": " + v.Msg
}

// UnprocessableEntityError represents a 422 Unprocessable Entity response.
// Errors holds the parsed validation failures, when the API returned them.
type UnprocessableEntityError struct {
	APIError
	Errors []ValidationError
}

func (e *UnprocessableEntityError) Unwrap() error { return &e.APIError }

// RateLimitError represents a 429 Too Many Requests response.
type RateLimitError struct{ APIError }

func (e *RateLimitError) Unwrap() error { return &e.APIError }

// InternalServerError represents a 5xx Server Error response.
type InternalServerError struct{ APIError }

func (e *InternalServerError) Unwrap() error { return &e.APIError }

// OverloadedError represents a 529 response: TypeSafe is temporarily overloaded.
// It is also an *InternalServerError, like every other 5xx.
type OverloadedError struct{ InternalServerError }

func (e *OverloadedError) Unwrap() error { return &e.InternalServerError }

// parseErrorBody extracts a message from an API error body. The API documents
// {"detail": string | object} for 400 and {"detail": [ValidationError]} for
// 422; "error" and "message" keys are accepted as fallbacks.
func parseErrorBody(body []byte) (string, []ValidationError) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return "", nil
	}
	for _, key := range []string{"detail", "error", "message"} {
		raw, ok := fields[key]
		if !ok || string(raw) == "null" {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if s == "" {
				continue
			}
			return s, nil
		}
		var ves []ValidationError
		if json.Unmarshal(raw, &ves) == nil {
			if len(ves) == 0 {
				continue
			}
			msgs := make([]string, len(ves))
			for i, ve := range ves {
				msgs[i] = ve.String()
			}
			return strings.Join(msgs, "; "), ves
		}
		return string(raw), nil
	}
	return "", nil
}

// NewAPIError returns an error of the appropriate type based on the status code.
func NewAPIError(statusCode int, body []byte, header http.Header, message string) error {
	var validation []ValidationError
	if len(body) > 0 {
		if parsed, ves := parseErrorBody(body); parsed != "" {
			message, validation = parsed, ves
		}
		if message == "" {
			message = string(body)
		}
	}

	base := APIError{
		StatusCode: statusCode,
		Body:       body,
		Header:     header,
		Message:    message,
	}

	switch statusCode {
	case 400:
		return &BadRequestError{base}
	case 401:
		return &AuthenticationError{base}
	case 403:
		return &PermissionDeniedError{base}
	case 404:
		return &NotFoundError{base}
	case 413:
		return &RequestTooLargeError{base}
	case 415:
		return &UnsupportedMediaTypeError{base}
	case 422:
		return &UnprocessableEntityError{APIError: base, Errors: validation}
	case 429:
		return &RateLimitError{base}
	case 529:
		return &OverloadedError{InternalServerError{base}}
	default:
		if statusCode >= 500 {
			return &InternalServerError{base}
		}
		return &base
	}
}

// TimeoutError represents a request timeout.
type TimeoutError struct {
	Err error
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("request timed out: %v", e.Err)
}

func (e *TimeoutError) Unwrap() error {
	return e.Err
}

// ConnectionError represents a failure to connect or read from the server.
type ConnectionError struct {
	Err error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("connection error: %v", e.Err)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// RetryPolicy configuration for SDK retry behavior.
type RetryPolicy struct {
	// Timeout is the total time budget for one SDK call, including the
	// initial attempt, retries and backoff delays. Zero means no limit.
	Timeout time.Duration

	MaxRetries int
	// BackoffInitial is the delay in seconds before the first retry; it doubles on each retry.
	BackoffInitial float64
	// BackoffMax caps the backoff delay in seconds. Zero means no cap.
	BackoffMax float64
	// BackoffJitter randomizes each delay by up to this fraction (0.25 = ±25%).
	BackoffJitter float64
	HTTPStatuses  []int
	// RespectRetryAfter honors the retry-after-ms and Retry-After response headers.
	RespectRetryAfter  bool
	APIConnectionError bool
	APITimeoutError    bool
}

// DefaultRetryPolicy returns the default retry configuration, matching the
// official SDKs: up to 2 retries of 408, 429 and every 5xx (including 529),
// within a 2 minute budget per call. The budget leaves room for every attempt
// to run to the client's 30 second per-attempt HTTP timeout, so timed-out
// attempts can still be retried.
func DefaultRetryPolicy() *RetryPolicy {
	statuses := []int{408, 429}
	for s := 500; s <= 599; s++ {
		statuses = append(statuses, s)
	}
	return &RetryPolicy{
		Timeout:            2 * time.Minute,
		MaxRetries:         2,
		BackoffInitial:     0.5,
		BackoffMax:         5.0,
		BackoffJitter:      0.25,
		HTTPStatuses:       statuses,
		RespectRetryAfter:  true,
		APIConnectionError: true,
		APITimeoutError:    true,
	}
}
