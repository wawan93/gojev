package gojev

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// UnprocessableEntityError represents a 422 Unprocessable Entity response.
type UnprocessableEntityError struct{ APIError }

func (e *UnprocessableEntityError) Unwrap() error { return &e.APIError }

// RateLimitError represents a 429 Too Many Requests response.
type RateLimitError struct{ APIError }

func (e *RateLimitError) Unwrap() error { return &e.APIError }

// InternalServerError represents a 5xx Server Error response.
type InternalServerError struct{ APIError }

func (e *InternalServerError) Unwrap() error { return &e.APIError }

// NewAPIError returns an error of the appropriate type based on the status code.
func NewAPIError(statusCode int, body []byte, header http.Header, message string) error {
	if len(body) > 0 {
		var parsed struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			if parsed.Error != "" {
				message = parsed.Error
			} else if parsed.Message != "" {
				message = parsed.Message
			}
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
	case 422:
		return &UnprocessableEntityError{base}
	case 429:
		return &RateLimitError{base}
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
	MaxRetries         int
	BackoffInitial     float64
	BackoffMax         float64
	BackoffJitter      float64
	HTTPStatuses       []int
	RespectRetryAfter  bool
	APIConnectionError bool
	APITimeoutError    bool
}

// DefaultRetryPolicy returns the default retry configuration.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:         2,
		BackoffInitial:     0.5,
		BackoffMax:         5.0,
		BackoffJitter:      0.25,
		HTTPStatuses:       []int{408, 429, 500, 501, 502, 503, 504, 505, 506, 507, 508, 510, 511},
		RespectRetryAfter:  true,
		APIConnectionError: true,
		APITimeoutError:    true,
	}
}
